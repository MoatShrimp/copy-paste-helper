// Package desktop is copy-paste-helper's only point of contact with the
// outside desktop environment: the clipboard, synthetic keystrokes, and
// notifications, all done by shelling out to small standard CLI tools
// (wl-clipboard, ydotool, wtype, notify-send) rather than linking against
// GTK/cgo.
package desktop

import (
	"bytes"
	"os/exec"
)

// GetClipboard reads the current clipboard text via wl-paste.
// An empty clipboard (wl-paste exits non-zero) is treated as "".
func GetClipboard() string {
	out, err := exec.Command("wl-paste", "-n").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// SetClipboard writes text to the clipboard via wl-copy.
func SetClipboard(text string) error {
	cmd := exec.Command("wl-copy")
	cmd.Stdin = bytes.NewReader([]byte(text))
	return cmd.Run()
}

func ClearClipboard() error {
	return exec.Command("wl-copy", "--clear").Run()
}
