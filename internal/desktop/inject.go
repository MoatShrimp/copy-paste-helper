package desktop

import (
	"errors"
	"sync"
)

var (
	keySenderMu sync.RWMutex
	keySender   func(...int) error
)

// SetKeySender installs the virtual-keyboard function used for shortcuts.
// The main package supplies the active platform keyboard listener; passing nil
// disables shortcut injection.
func SetKeySender(sender func(...int) error) {
	keySenderMu.Lock()
	keySender = sender
	keySenderMu.Unlock()
}

func sendKeyCombo(codes ...int) error {
	keySenderMu.RLock()
	sender := keySender
	keySenderMu.RUnlock()
	if sender == nil {
		return errors.New("virtual keyboard is not initialized")
	}
	return sender(codes...)
}

func SendCtrlC() error {
	return sendKeyCombo(keyLeftCtrl, keyC)
}

func SendCtrlV() error {
	return sendKeyCombo(keyLeftCtrl, keyV)
}
