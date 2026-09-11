//go:build windows

package desktop

import "copy-paste-helper/internal/keyboard"

// Windows virtual-key codes used by SendInput.
const (
	keyLeftCtrl = 0xa2 // VK_LCONTROL
	keyC        = 0x43 // C
	keyV        = 0x56 // V
)

// TypeText uses KEYEVENTF_UNICODE so text entry is independent of the active
// Windows keyboard layout.
func TypeText(text string) error {
	return keyboard.TypeText(text)
}
