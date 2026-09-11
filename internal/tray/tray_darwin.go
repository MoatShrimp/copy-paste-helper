//go:build darwin

// Package tray supplies a buildable placeholder on macOS. Avoiding the native
// tray backend here keeps shared package tests linkable alongside the clipboard
// backend while global keyboard capture remains unsupported.
package tray

import (
	"errors"
	"sync/atomic"

	"copy-paste-helper/internal/engine"
)

var errUnsupported = errors.New("system tray is unavailable in the macOS test build")

type Tray struct{}

func New(*engine.State, *atomic.Bool, chan func(), func()) *Tray { return &Tray{} }

func (*Tray) Run() error { return errUnsupported }

func (*Tray) Remove() {}

func (*Tray) Sync(*engine.State, bool) {}
