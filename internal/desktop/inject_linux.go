//go:build linux

package desktop

import "os/exec"

// Linux input-event-codes.h keycodes used for synthetic key injection.
const (
	keyLeftCtrl = 29
	keyC        = 46
	keyV        = 47
)

// TypeText types text out as literal keystrokes, without touching the
// clipboard. Direct uinput injection only knows physical keycodes and cannot
// map arbitrary Unicode through the active keyboard layout; wtype builds a
// keymap for the exact characters being typed instead.
func TypeText(text string) error {
	if text == "" {
		return nil
	}
	return exec.Command("wtype", "--", text).Run()
}
