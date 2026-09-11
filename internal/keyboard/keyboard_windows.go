//go:build windows

// Package keyboard captures the application's hotkeys with a Windows
// low-level keyboard hook. Unlike RegisterHotKey, the hook supports bare keys
// and can suppress them before they reach the foreground application.
package keyboard

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL = 13

	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	wmQuit       = 0x0012

	llkhfInjected = 0x10
	llkhfExtended = 0x01
	pmNoRemove    = 0x0000

	inputKeyboard    = 1
	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004

	vkControl  = 0x11
	vkLControl = 0xa2
	vkRControl = 0xa3
	vkReturn   = 0x0d
	vkTab      = 0x09
)

var HotkeyToSlot = map[int]string{
	0x70: "f1", 0x71: "f2", 0x72: "f3", 0x73: "f4", 0x74: "f5", 0x75: "f6",
	0x76: "f7", 0x77: "f8", 0x78: "f9", 0x79: "f10", 0x7a: "f11", 0x7b: "f12",
	0x61: "n1", 0x62: "n2", 0x63: "n3", 0x64: "n4", 0x65: "n5", 0x66: "n6",
	0x67: "n7", 0x68: "n8", 0x69: "n9",
}

const (
	KeyKPPlus     = 0x6b // VK_ADD
	KeyKPMinus    = 0x6d // VK_SUBTRACT
	KeyKPSlash    = 0x6f // VK_DIVIDE
	KeyKPAsterisk = 0x6a // VK_MULTIPLY
	KeyKP0        = 0x60 // VK_NUMPAD0
	KeyKPDot      = 0x6e // VK_DECIMAL
	KeyPause      = 0x13 // VK_PAUSE
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

type KeyEvent struct {
	Code    int
	Pressed bool
	Ctrl    bool
}

type kbdLLHookStruct struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type point struct {
	X int32
	Y int32
}

type message struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
}

type keyboardInput struct {
	VK        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPeekMessageW        = user32.NewProc("PeekMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procSendInput           = user32.NewProc("SendInput")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

// FindDevices returns one logical source because WH_KEYBOARD_LL observes all
// keyboards in the interactive Windows desktop session.
func FindDevices() ([]string, error) {
	return []string{"Windows keyboard hook"}, nil
}

type Listener struct {
	Events    chan KeyEvent
	suspended *atomic.Bool

	hook      uintptr
	callback  uintptr
	threadID  uint32
	ready     chan error
	done      chan struct{}
	closeOnce sync.Once
	pressed   map[int]bool
	leftCtrl  bool
	rightCtrl bool
}

func Open(_ string, suspended *atomic.Bool) (*Listener, error) {
	kl := &Listener{
		Events:    make(chan KeyEvent, 64),
		suspended: suspended,
		ready:     make(chan error, 1),
		done:      make(chan struct{}),
		pressed:   make(map[int]bool),
	}
	go kl.run()
	if err := <-kl.ready; err != nil {
		return nil, err
	}
	return kl, nil
}

func (kl *Listener) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(kl.done)
	defer close(kl.Events)

	kl.threadID = windows.GetCurrentThreadId()
	var msg message
	// Force creation of this thread's message queue before Close can post
	// WM_QUIT to it.
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmNoRemove)

	kl.callback = windows.NewCallback(kl.hookProc)
	module, _, _ := procGetModuleHandleW.Call(0)
	hook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, kl.callback, module, 0)
	if hook == 0 {
		kl.ready <- windowsCallError("SetWindowsHookExW", callErr)
		return
	}
	kl.hook = hook
	kl.ready <- nil

	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			_ = callErr
			procUnhookWindowsHookEx.Call(kl.hook)
			return
		case 0:
			procUnhookWindowsHookEx.Call(kl.hook)
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (kl *Listener) hookProc(nCode, wParam uintptr, data *kbdLLHookStruct) uintptr {
	if int32(nCode) < 0 {
		return kl.callNext(nCode, wParam, data)
	}

	if data.Flags&llkhfInjected != 0 {
		return kl.callNext(nCode, wParam, data)
	}

	rawCode := int(data.VKCode)
	pressed, isKeyMessage := keyMessageState(uint32(wParam))
	if !isKeyMessage {
		return kl.callNext(nCode, wParam, data)
	}

	switch rawCode {
	case vkControl:
		if data.Flags&llkhfExtended != 0 {
			kl.rightCtrl = pressed
		} else {
			kl.leftCtrl = pressed
		}
		return kl.callNext(nCode, wParam, data)
	case vkLControl:
		kl.leftCtrl = pressed
		return kl.callNext(nCode, wParam, data)
	case vkRControl:
		kl.rightCtrl = pressed
		return kl.callNext(nCode, wParam, data)
	}
	code := normalizeKeyCode(rawCode, data.ScanCode, data.Flags)

	wasPressed := kl.pressed[code]
	intercept := code == KeyPause || wasPressed || (kl.captureEnabled() && isHotkey(code))
	if !intercept {
		return kl.callNext(nCode, wParam, data)
	}

	if pressed && !wasPressed {
		kl.pressed[code] = true
		kl.emit(KeyEvent{Code: code, Pressed: true, Ctrl: kl.leftCtrl || kl.rightCtrl})
	} else if !pressed && wasPressed {
		delete(kl.pressed, code)
		kl.emit(KeyEvent{Code: code, Pressed: false, Ctrl: kl.leftCtrl || kl.rightCtrl})
	}
	return 1
}

// normalizeKeyCode keeps numpad bindings physical and independent of Num
// Lock. Windows reports the navigation VK codes for these scan codes when Num
// Lock is off; non-keypad navigation keys carry the extended flag instead.
func normalizeKeyCode(vk int, scanCode, flags uint32) int {
	if vk == 0x03 { // VK_CANCEL, emitted by Ctrl+Pause/Break
		return KeyPause
	}
	if flags&llkhfExtended != 0 {
		if scanCode == 0x35 {
			return KeyKPSlash
		}
		return vk
	}
	switch scanCode {
	case 0x47:
		return 0x67 // VK_NUMPAD7
	case 0x48:
		return 0x68 // VK_NUMPAD8
	case 0x49:
		return 0x69 // VK_NUMPAD9
	case 0x4a:
		return KeyKPMinus
	case 0x4b:
		return 0x64 // VK_NUMPAD4
	case 0x4c:
		return 0x65 // VK_NUMPAD5
	case 0x4d:
		return 0x66 // VK_NUMPAD6
	case 0x4e:
		return KeyKPPlus
	case 0x4f:
		return 0x61 // VK_NUMPAD1
	case 0x50:
		return 0x62 // VK_NUMPAD2
	case 0x51:
		return 0x63 // VK_NUMPAD3
	case 0x52:
		return KeyKP0
	case 0x53:
		return KeyKPDot
	case 0x37:
		return KeyKPAsterisk
	default:
		return vk
	}
}

func (kl *Listener) captureEnabled() bool {
	return kl.suspended == nil || !kl.suspended.Load()
}

func (kl *Listener) emit(event KeyEvent) {
	select {
	case kl.Events <- event:
	default:
		// A Windows hook must return promptly or the OS removes it. Dropping an
		// event under extreme backpressure is safer than losing the hook.
	}
}

func (kl *Listener) callNext(nCode, wParam uintptr, data *kbdLLHookStruct) uintptr {
	result, _, _ := procCallNextHookEx.Call(kl.hook, nCode, wParam, uintptr(unsafe.Pointer(data)))
	return result
}

func keyMessageState(message uint32) (pressed, ok bool) {
	switch message {
	case wmKeyDown, wmSysKeyDown:
		return true, true
	case wmKeyUp, wmSysKeyUp:
		return false, true
	default:
		return false, false
	}
}

func (kl *Listener) Close() {
	kl.closeOnce.Do(func() {
		procPostThreadMessageW.Call(uintptr(kl.threadID), wmQuit, 0, 0)
		<-kl.done
	})
}

func (kl *Listener) SendKeyCombo(codes ...int) error {
	inputs := make([]input, 0, len(codes)*2)
	for _, code := range codes {
		inputs = append(inputs, newKeyboardInput(uint16(code), 0, 0))
	}
	for i := len(codes) - 1; i >= 0; i-- {
		inputs = append(inputs, newKeyboardInput(uint16(codes[i]), 0, keyeventfKeyUp))
	}
	return sendInputs(inputs)
}

// TypeText injects UTF-16 code units with KEYEVENTF_UNICODE, bypassing the
// active keyboard layout while preserving the exact configured text.
func TypeText(text string) error {
	return sendInputs(textInputs(text))
}

func textInputs(text string) []input {
	inputs := make([]input, 0, len(text)*2)
	runes := []rune(text)
	for i, r := range runes {
		if r == '\n' && i > 0 && runes[i-1] == '\r' {
			continue
		}
		switch r {
		case '\r', '\n':
			inputs = append(inputs,
				newKeyboardInput(vkReturn, 0, 0),
				newKeyboardInput(vkReturn, 0, keyeventfKeyUp),
			)
		case '\t':
			inputs = append(inputs,
				newKeyboardInput(vkTab, 0, 0),
				newKeyboardInput(vkTab, 0, keyeventfKeyUp),
			)
		default:
			for _, unit := range utf16.Encode([]rune{r}) {
				inputs = append(inputs,
					newKeyboardInput(0, unit, keyeventfUnicode),
					newKeyboardInput(0, unit, keyeventfUnicode|keyeventfKeyUp),
				)
			}
		}
	}
	return inputs
}

func newKeyboardInput(vk, scan uint16, flags uint32) input {
	in := input{Type: inputKeyboard}
	ki := (*keyboardInput)(unsafe.Pointer(&in.Data[0]))
	ki.VK = vk
	ki.Scan = scan
	ki.Flags = flags
	return in
}

func sendInputs(inputs []input) error {
	if len(inputs) == 0 {
		return nil
	}
	written, _, callErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(input{}),
	)
	if int(written) != len(inputs) {
		return windowsCallError(fmt.Sprintf("SendInput wrote %d of %d events", written, len(inputs)), callErr)
	}
	return nil
}

func windowsCallError(operation string, err error) error {
	if err == nil || err == windows.ERROR_SUCCESS {
		return fmt.Errorf("%s failed", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
