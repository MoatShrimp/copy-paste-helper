// Package tray is the right-click system tray icon: a Template submenu, a
// Mode submenu, a Suspended toggle, and Quit, backed by the
// StatusNotifierItem D-Bus protocol (github.com/gogpu/systray) rather than
// GTK, so it needs no cgo.
package tray

import (
	"sync/atomic"

	"github.com/gogpu/systray"

	"copy-paste-helper/internal/engine"
)

// Tray mirrors and controls engine.State. Every click handler only ever
// pushes a closure onto commands — actual state mutation happens on the
// caller's event loop (the same place keyboard hotkeys are handled), so
// State never needs its own locking.
type Tray struct {
	st            *systray.SystemTray
	templateItems []*systray.MenuItem
	writeItem     *systray.MenuItem
	pasteItem     *systray.MenuItem
	suspendedItem *systray.MenuItem
}

// New builds the tray icon and menu. commands is where click handlers post
// state-mutating closures for the caller to run; quit is called for the
// Quit menu item.
func New(state *engine.State, suspended *atomic.Bool, commands chan func(), quit func()) *Tray {
	tr := &Tray{}

	menu := systray.NewMenu()

	templateMenu := systray.NewMenu()
	for i, t := range state.Templates() {
		idx := i
		item := templateMenu.AddCheckbox(t.Name, i == state.TemplateIndex(), func() {
			commands <- func() { state.SwitchToTemplate(idx) }
		})
		tr.templateItems = append(tr.templateItems, item)
	}
	menu.AddSubmenu("Template", templateMenu)

	modeMenu := systray.NewMenu()
	tr.writeItem = modeMenu.AddCheckbox("Write", !state.PasteMode(), func() {
		commands <- func() { state.ToggleMode(false) }
	})
	tr.pasteItem = modeMenu.AddCheckbox("Paste", state.PasteMode(), func() {
		commands <- func() { state.ToggleMode(true) }
	})
	menu.AddSubmenu("Mode", modeMenu)

	menu.AddSeparator()
	tr.suspendedItem = menu.AddCheckbox("Suspended", suspended.Load(), func() {
		commands <- func() { engine.ToggleSuspend(suspended) }
	})

	menu.AddSeparator()
	menu.Add("Quit", func() { quit() })

	tr.st = systray.New().
		SetIcon(iconActivePNG).
		SetTooltip("copy-paste-helper — " + state.Current().Name).
		SetMenu(menu).
		Show()

	return tr
}

// Run pumps the tray's event loop; it blocks until Remove is called.
func (tr *Tray) Run() error { return tr.st.Run() }

// Remove destroys the tray icon and unblocks Run.
func (tr *Tray) Remove() { tr.st.Remove() }

// Sync refreshes every checkbox and the icon/tooltip to match current
// state. Called after any command or hotkey that might have changed
// something, whichever goroutine handled it.
func (tr *Tray) Sync(state *engine.State, suspended bool) {
	for i, item := range tr.templateItems {
		item.SetChecked(i == state.TemplateIndex())
	}
	tr.writeItem.SetChecked(!state.PasteMode())
	tr.pasteItem.SetChecked(state.PasteMode())
	tr.suspendedItem.SetChecked(suspended)

	if suspended {
		tr.st.SetIcon(iconSuspendedPNG).SetTooltip("copy-paste-helper — suspended")
	} else {
		tr.st.SetIcon(iconActivePNG).SetTooltip("copy-paste-helper — " + state.Current().Name)
	}
}
