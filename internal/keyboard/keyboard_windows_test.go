//go:build windows

package keyboard

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsInputLayout(t *testing.T) {
	wantInput, wantKeyboardInput := uintptr(40), uintptr(24)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantInput, wantKeyboardInput = 28, 16
	}
	if got := unsafe.Sizeof(input{}); got != wantInput {
		t.Fatalf("sizeof(INPUT) = %d, want %d", got, wantInput)
	}
	if got := unsafe.Sizeof(keyboardInput{}); got != wantKeyboardInput {
		t.Fatalf("sizeof(KEYBDINPUT) = %d, want %d", got, wantKeyboardInput)
	}
	kl := &Listener{}
	if callback := windows.NewCallback(kl.hookProc); callback == 0 {
		t.Fatal("NewCallback returned a null hook procedure")
	}
}

func TestNormalizeNumpadKeyIgnoresNumLock(t *testing.T) {
	tests := []struct {
		name  string
		vk    int
		scan  uint32
		flags uint32
		want  int
	}{
		{name: "numpad 1 with num lock", vk: 0x61, scan: 0x4f, want: 0x61},
		{name: "numpad 1 without num lock", vk: 0x23, scan: 0x4f, want: 0x61},
		{name: "dedicated End", vk: 0x23, scan: 0x4f, flags: llkhfExtended, want: 0x23},
		{name: "numpad divide", vk: 0x6f, scan: 0x35, flags: llkhfExtended, want: KeyKPSlash},
		{name: "control pause", vk: 0x03, scan: 0x46, want: KeyPause},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeKeyCode(tt.vk, tt.scan, tt.flags); got != tt.want {
				t.Fatalf("normalizeKeyCode(%#x, %#x, %#x) = %#x, want %#x", tt.vk, tt.scan, tt.flags, got, tt.want)
			}
		})
	}
}

func TestTextInputsTreatsNewlinesAndTabsAsKeys(t *testing.T) {
	inputs := textInputs("A\r\n\t😀")
	// A = 2 events, CRLF = one Enter pair, tab = one pair, and the emoji's
	// surrogate pair = 4 events.
	if got, want := len(inputs), 10; got != want {
		t.Fatalf("textInputs event count = %d, want %d", got, want)
	}

	enter := (*keyboardInput)(unsafe.Pointer(&inputs[2].Data[0]))
	if enter.VK != vkReturn || enter.Flags != 0 {
		t.Fatalf("newline keydown = {VK:%#x Flags:%#x}, want VK_RETURN keydown", enter.VK, enter.Flags)
	}
	tab := (*keyboardInput)(unsafe.Pointer(&inputs[4].Data[0]))
	if tab.VK != vkTab || tab.Flags != 0 {
		t.Fatalf("tab keydown = {VK:%#x Flags:%#x}, want VK_TAB keydown", tab.VK, tab.Flags)
	}
}

func TestKeyMessageState(t *testing.T) {
	if pressed, ok := keyMessageState(wmKeyDown); !ok || !pressed {
		t.Fatalf("WM_KEYDOWN = (%v, %v), want (true, true)", pressed, ok)
	}
	if pressed, ok := keyMessageState(wmKeyUp); !ok || pressed {
		t.Fatalf("WM_KEYUP = (%v, %v), want (false, true)", pressed, ok)
	}
	if _, ok := keyMessageState(0); ok {
		t.Fatal("unknown message reported as a key message")
	}
}
