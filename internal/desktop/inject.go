package desktop

import (
	"fmt"
	"os/exec"
)

// Linux input-event-codes.h keycodes used for synthetic key injection.
const (
	keyLeftCtrl = 29
	keyC        = 46
	keyV        = 47
)

func sendKeyCombo(codes ...int) error {
	args := make([]string, 0, len(codes)*2)
	for _, c := range codes {
		args = append(args, fmt.Sprintf("%d:1", c))
	}
	for i := len(codes) - 1; i >= 0; i-- {
		args = append(args, fmt.Sprintf("%d:0", codes[i]))
	}
	return exec.Command("ydotool", append([]string{"key"}, args...)...).Run()
}

func SendCtrlC() error {
	return sendKeyCombo(keyLeftCtrl, keyC)
}

func SendCtrlV() error {
	return sendKeyCombo(keyLeftCtrl, keyV)
}

// TypeText types text out as literal keystrokes, without touching the
// clipboard. This uses wtype rather than `ydotool type`: ydotool simulates
// fixed US-layout keycodes, which come out wrong on any other active
// keyboard layout (e.g. a Swedish layout renders some punctuation as
// letters with diacritics); wtype instead builds a keymap for the exact
// characters being typed, so it's correct regardless of layout.
func TypeText(text string) error {
	if text == "" {
		return nil
	}
	return exec.Command("wtype", "--", text).Run()
}
