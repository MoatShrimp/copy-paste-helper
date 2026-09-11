//go:build !linux && !windows

package desktop

import "errors"

const (
	keyLeftCtrl = 0
	keyC        = 0
	keyV        = 0
)

func TypeText(string) error {
	return errors.New("text injection is only supported on Linux and Windows")
}
