//go:build !linux && !windows

// Package keyboard provides the application's global-hotkey boundary. Linux
// and Windows have native implementations; this file keeps the application
// and its tests buildable on other platforms.
package keyboard

import (
	"errors"
	"sync/atomic"
)

var ErrUnsupported = errors.New("global keyboard capture is only supported on Linux and Windows")

var HotkeyToSlot = map[int]string{
	59: "f1", 60: "f2", 61: "f3", 62: "f4", 63: "f5", 64: "f6",
	65: "f7", 66: "f8", 67: "f9", 68: "f10", 87: "f11", 88: "f12",
	79: "n1", 80: "n2", 81: "n3", 75: "n4", 76: "n5", 77: "n6",
	71: "n7", 72: "n8", 73: "n9",
}

const (
	KeyKPPlus     = 78
	KeyKPMinus    = 74
	KeyKPSlash    = 98
	KeyKPAsterisk = 55
	KeyKP0        = 82
	KeyKPDot      = 83
	KeyPause      = 119
)

type KeyEvent struct {
	Code    int
	Pressed bool
	Ctrl    bool
}

type Listener struct {
	Events chan KeyEvent
}

func FindDevices() ([]string, error) {
	return nil, ErrUnsupported
}

func Open(string, *atomic.Bool) (*Listener, error) {
	return nil, ErrUnsupported
}

func (kl *Listener) Close() {}

func (kl *Listener) SendKeyCombo(...int) error {
	return ErrUnsupported
}
