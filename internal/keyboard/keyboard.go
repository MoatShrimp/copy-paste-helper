//go:build linux

// Package keyboard grabs a keyboard device exclusively via evdev, re-emits
// every key that isn't one of copy-paste-helper's hotkeys through a virtual
// uinput device (so the keyboard keeps working normally), and delivers
// hotkey presses as KeyEvents. It doesn't know what those hotkeys do —
// that's the engine package's job — only which raw keycodes to intercept.
package keyboard

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Linux input subsystem constants (linux/input-event-codes.h, linux/input.h,
// linux/uinput.h). Reproduced here to avoid a cgo/x-sys dependency.
const (
	evSyn = 0x00
	evKey = 0x01

	synReport = 0

	keyMax = 0x2ff
	keyA   = 30

	keyLeftCtrlCode  = 29
	keyRightCtrlCode = 97

	inputEventSize = 24 // sizeof(struct input_event) on linux/amd64

	uinputMaxNameSize = 80
	absCnt            = 64
)

// HotkeyToSlot maps evdev keycodes to the button key they control.
var HotkeyToSlot = map[int]string{
	59: "f1", 60: "f2", 61: "f3", 62: "f4", 63: "f5", 64: "f6",
	65: "f7", 66: "f8", 67: "f9", 68: "f10", 87: "f11", 88: "f12",
	79: "n1", 80: "n2", 81: "n3", 75: "n4", 76: "n5", 77: "n6",
	71: "n7", 72: "n8", 73: "n9",
}

// The non-slot hotkeys: template switching, mode toggling, info, flush-all,
// and suspend. Exported so the engine package can dispatch on them.
const (
	KeyKPPlus     = 78
	KeyKPMinus    = 74
	KeyKPSlash    = 98
	KeyKPAsterisk = 55
	KeyKP0        = 82
	KeyKPDot      = 83
	KeyPause      = 119
)

func isControlHotkey(code int) bool {
	switch code {
	case KeyKPPlus, KeyKPMinus, KeyKPSlash, KeyKPAsterisk, KeyKP0, KeyKPDot, KeyPause:
		return true
	}
	return false
}

func isHotkey(code int) bool {
	if _, ok := HotkeyToSlot[code]; ok {
		return true
	}
	return isControlHotkey(code)
}

// KeyEvent is the decoded, application-relevant subset of an input_event.
type KeyEvent struct {
	Code    int
	Pressed bool // true = down, false = up (repeats are dropped before this point)
	Ctrl    bool
}

// ioc computes a Linux ioctl request number (see asm-generic/ioctl.h).
func ioc(dir, typ, nr, size uintptr) uintptr {
	return (dir << 30) | (typ << 8) | nr | (size << 16)
}

const (
	iocNone  = 0
	iocWrite = 1
	iocRead  = 2
)

func ioctl(fd uintptr, req uintptr, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	if errno != 0 {
		return errno
	}
	return nil
}

func eviocgrab(fd uintptr, grab bool) error {
	req := ioc(iocWrite, 'E', 0x90, 4)
	v := 0
	if grab {
		v = 1
	}
	// Like UI_SET_EVBIT, EVIOCGRAB takes the value directly as the ioctl
	// argument, not a pointer to it, despite the _IOW(..., int) declaration.
	return ioctl(fd, req, uintptr(v))
}

func eviocgbitKey(fd uintptr, buf []byte) error {
	req := ioc(iocRead, 'E', 0x20+evKey, uintptr(len(buf)))
	return ioctl(fd, req, uintptr(unsafe.Pointer(&buf[0])))
}

func bitSet(buf []byte, bit int) bool {
	idx := bit / 8
	if idx >= len(buf) {
		return false
	}
	return buf[idx]&(1<<uint(bit%8)) != 0
}

// FindDevices returns evdev device paths that look like a real keyboard
// (support EV_KEY and have a KEY_A key), which filters out mice, power
// buttons, and similar single-purpose input devices.
func FindDevices() ([]string, error) {
	matches, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, err
	}
	var result []string
	for _, path := range matches {
		f, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			continue // typically permission denied on devices we don't care about
		}
		buf := make([]byte, (keyMax/8)+1)
		err = eviocgbitKey(f.Fd(), buf)
		f.Close()
		if err != nil {
			continue
		}
		if bitSet(buf, keyA) {
			result = append(result, path)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no keyboard-like device found under /dev/input (check that your user is in the 'input' group)")
	}
	return result, nil
}

// Listener grabs one keyboard device exclusively, re-emits every key that
// isn't one of our hotkeys through a virtual uinput device (so the
// keyboard keeps working normally), and delivers hotkey presses on Events.
type Listener struct {
	src       *os.File
	uinput    *os.File
	uinputMu  sync.Mutex
	Events    chan KeyEvent
	suspended *atomic.Bool
}

// Open grabs the keyboard device at path. suspended is consulted on every
// event: while true, only Pause is intercepted (everything else, including
// the other hotkeys, passes through untouched) so the keyboard behaves
// normally while copy-paste-helper is suspended.
func Open(path string, suspended *atomic.Bool) (*Listener, error) {
	src, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	keyBits := make([]byte, (keyMax/8)+1)
	if err := eviocgbitKey(src.Fd(), keyBits); err != nil {
		src.Close()
		return nil, fmt.Errorf("query key bits: %w", err)
	}

	uinput, err := createPassthroughDevice(keyBits, filepath.Base(path))
	if err != nil {
		src.Close()
		return nil, err
	}

	if err := eviocgrab(src.Fd(), true); err != nil {
		src.Close()
		uinput.Close()
		return nil, fmt.Errorf("grab %s: %w (is your user in the 'input' group?)", path, err)
	}

	kl := &Listener{src: src, uinput: uinput, Events: make(chan KeyEvent, 16), suspended: suspended}
	go kl.readLoop()
	return kl, nil
}

func (kl *Listener) Close() {
	_ = eviocgrab(kl.src.Fd(), false)
	_ = destroyUinputDevice(kl.uinput.Fd())
	kl.src.Close()
	kl.uinput.Close()
}

// SendKeyCombo emits a chord through the listener's virtual keyboard. All keys
// are pressed in order and released in reverse order, matching the convention
// used for modifier shortcuts such as Ctrl+C and Ctrl+V.
func (kl *Listener) SendKeyCombo(codes ...int) error {
	kl.uinputMu.Lock()
	defer kl.uinputMu.Unlock()

	for _, code := range codes {
		if err := kl.writeInputEvent(evKey, code, 1); err != nil {
			return err
		}
	}
	if err := kl.writeInputEvent(evSyn, synReport, 0); err != nil {
		return err
	}
	for i := len(codes) - 1; i >= 0; i-- {
		if err := kl.writeInputEvent(evKey, codes[i], 0); err != nil {
			return err
		}
	}
	return kl.writeInputEvent(evSyn, synReport, 0)
}

func (kl *Listener) writeInputEvent(eventType, code int, value int32) error {
	buf := make([]byte, inputEventSize)
	binary.LittleEndian.PutUint16(buf[16:18], uint16(eventType))
	binary.LittleEndian.PutUint16(buf[18:20], uint16(code))
	binary.LittleEndian.PutUint32(buf[20:24], uint32(value))
	_, err := kl.uinput.Write(buf)
	return err
}

func (kl *Listener) readLoop() {
	buf := make([]byte, inputEventSize)
	ctrl := false
	for {
		n, err := kl.src.Read(buf)
		if err != nil || n != inputEventSize {
			close(kl.Events)
			return
		}

		evType := binary.LittleEndian.Uint16(buf[16:18])
		code := int(binary.LittleEndian.Uint16(buf[18:20]))
		value := int32(binary.LittleEndian.Uint32(buf[20:24]))

		if evType == evKey && (code == keyLeftCtrlCode || code == keyRightCtrlCode) {
			ctrl = value != 0
		}

		// Pause always reaches us so it can toggle suspend / quit even while
		// suspended; every other hotkey passes through untouched when suspended.
		intercept := evType == evKey && (code == KeyPause || (!kl.suspended.Load() && isHotkey(code)))

		if intercept {
			if value == 0 || value == 1 { // ignore auto-repeat (value 2)
				kl.Events <- KeyEvent{Code: code, Pressed: value == 1, Ctrl: ctrl}
			}
			continue // swallow: don't forward hotkeys to the passthrough device
		}

		kl.uinputMu.Lock()
		_, err = kl.uinput.Write(buf)
		kl.uinputMu.Unlock()
		if err != nil {
			close(kl.Events)
			return
		}
	}
}

// --- uinput virtual device creation ---

func createPassthroughDevice(sourceKeyBits []byte, sourceName string) (*os.File, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/uinput: %w (check permissions / kernel module)", err)
	}

	uiSetEvbit := ioc(iocWrite, 'U', 100, 4)
	uiSetKeybit := ioc(iocWrite, 'U', 101, 4)
	uiDevCreate := ioc(iocNone, 'U', 1, 0)

	// UI_SET_EVBIT/UI_SET_KEYBIT are declared as _IOW(..., int) but the
	// uinput driver actually reads the ioctl argument itself as the bit
	// number, not as a pointer to an int — pass the value directly.
	setInt := func(req uintptr, v int) error {
		return ioctl(f.Fd(), req, uintptr(v))
	}

	if err := setInt(uiSetEvbit, evSyn); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_SET_EVBIT(EV_SYN): %w", err)
	}
	if err := setInt(uiSetEvbit, evKey); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_SET_EVBIT(EV_KEY): %w", err)
	}
	for code := 0; code <= keyMax; code++ {
		if bitSet(sourceKeyBits, code) {
			if err := setInt(uiSetKeybit, code); err != nil {
				f.Close()
				return nil, fmt.Errorf("UI_SET_KEYBIT(%d): %w", code, err)
			}
		}
	}

	dev := makeUinputUserDev("copy-paste-helper (" + sourceName + ")")
	if _, err := f.Write(dev); err != nil {
		f.Close()
		return nil, fmt.Errorf("write uinput_user_dev: %w", err)
	}

	if err := ioctl(f.Fd(), uiDevCreate, 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_CREATE: %w", err)
	}

	time.Sleep(200 * time.Millisecond) // let udev settle before we start writing events
	return f, nil
}

func destroyUinputDevice(fd uintptr) error {
	const uiDevDestroy = 0x5502 // _IO('U', 2)
	return ioctl(fd, uiDevDestroy, 0)
}

// makeUinputUserDev builds the legacy `struct uinput_user_dev` byte layout:
// char name[80]; struct input_id id (4x uint16); __u32 ff_effects_max;
// __s32 absmax/absmin/absfuzz/absflat[64].
func makeUinputUserDev(name string) []byte {
	size := uinputMaxNameSize + 8 + 4 + 4*absCnt*4
	buf := make([]byte, size)
	copy(buf[0:uinputMaxNameSize], []byte(name))

	off := uinputMaxNameSize
	binary.LittleEndian.PutUint16(buf[off:], 0x06) // bustype = BUS_VIRTUAL
	binary.LittleEndian.PutUint16(buf[off+2:], 1)  // vendor
	binary.LittleEndian.PutUint16(buf[off+4:], 1)  // product
	binary.LittleEndian.PutUint16(buf[off+6:], 1)  // version
	// ff_effects_max and the abs* arrays are left zeroed; we don't use them.
	return buf
}
