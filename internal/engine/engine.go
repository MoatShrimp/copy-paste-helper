// Package engine is the core copy-paste-helper state machine: which
// template is active, the scratch (copy/paste) buffers for buttons with no
// template text, and the current paste mode. It reacts to input from both
// the keyboard package (hotkeys) and the tray package (menu clicks),
// treating them identically — both just call into State.
package engine

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"copy-paste-helper/internal/desktop"
	"copy-paste-helper/internal/keyboard"
	"copy-paste-helper/internal/template"
)

// State holds all of the mutable runtime state: which template is active,
// the scratch (copy/paste) buffers for slots that have no template text,
// and the current paste mode.
type State struct {
	templates []*template.Template
	tplIndex  int

	scratch   map[string]string
	pasteMode bool
}

func NewState(templates []*template.Template) *State {
	return &State{
		templates: templates,
		scratch:   map[string]string{},
	}
}

// Templates and TemplateIndex expose the loaded template list and current
// selection for UI code (the tray menu) to render.
func (s *State) Templates() []*template.Template { return s.templates }
func (s *State) TemplateIndex() int              { return s.tplIndex }
func (s *State) PasteMode() bool                 { return s.pasteMode }

func (s *State) Current() *template.Template {
	return s.templates[s.tplIndex]
}

func (s *State) resetScratch() {
	s.scratch = map[string]string{}
}

// ReplaceTemplates swaps in a freshly loaded and validated template list —
// used when the templates directory changes on disk (e.g. the editor GUI
// saved a change) and should take effect without restarting. The
// currently active template stays selected, with its scratch buffers
// intact, if a template at the same Path still exists in the new list;
// otherwise selection falls back to the first template with scratch reset.
// A nil or empty list is ignored, since it usually means every template
// file is mid-write rather than genuinely all gone.
func (s *State) ReplaceTemplates(templates []*template.Template) {
	if len(templates) == 0 {
		return
	}
	currentPath := s.Current().Path
	s.templates = templates
	for i, t := range templates {
		if t.Path == currentPath {
			s.tplIndex = i
			return
		}
	}
	s.tplIndex = 0
	s.resetScratch()
}

func (s *State) SwitchTemplate(delta int) {
	next := s.tplIndex + delta
	if next < 0 || next >= len(s.templates) {
		desktop.Notify("Template", fmt.Sprintf("<b>%s</b>\n<i>(no more templates in that direction)</i>", desktop.EscapeMarkup(s.Current().Name)))
		return
	}
	s.SwitchToTemplate(next)
}

// SwitchToTemplate jumps directly to the template at idx (used by the
// tray's Template submenu, which lets you pick any template rather than
// stepping through them one at a time).
func (s *State) SwitchToTemplate(idx int) {
	if idx < 0 || idx >= len(s.templates) || idx == s.tplIndex {
		return
	}
	s.tplIndex = idx
	s.resetScratch()
	desktop.Notify("Template Switched", fmt.Sprintf("<b>%s</b>", desktop.EscapeMarkup(s.Current().Name)))
}

func (s *State) ToggleMode(paste bool) {
	s.pasteMode = paste
	if paste {
		desktop.Notify("Mode: Paste", "Buttons paste via the clipboard (<b>Ctrl+V</b>)")
	} else {
		desktop.Notify("Mode: Write", "Buttons type text out directly")
	}
}

func (s *State) ShowInfo() {
	t := s.Current()
	var lines []string
	for _, key := range template.SlotOrder {
		b, ok := t.ByKey(key)
		if !ok {
			continue // this template doesn't bind this key at all
		}
		keyLabel := fmt.Sprintf("<b>%s</b>", strings.ToUpper(key))

		if b.Text != "" {
			label := b.ToolTip
			if label == "" {
				label = fmt.Sprintf("%s  %s", keyLabel, desktop.EscapeMarkup(firstLine(s.resolveText(t, b.Text))))
			}
			lines = append(lines, label)
			continue
		}

		status := "<i>empty</i>"
		if v, ok := s.scratch[key]; ok {
			status = "stored: " + desktop.EscapeMarkup(firstLine(v))
		}
		if b.ToolTip != "" {
			lines = append(lines, fmt.Sprintf("%s — %s", b.ToolTip, status))
		} else {
			lines = append(lines, fmt.Sprintf("%s  %s", keyLabel, status))
		}
	}
	if len(lines) == 0 {
		lines = []string{"<i>No buttons are defined in this template.</i>"}
	}
	desktop.NotifyLines(t.Name, lines)
}

func (s *State) FlushAll() {
	s.resetScratch()
	desktop.Notify("Flushed", "All scratch buffers have been cleared")
}

// CopyPaste implements the core CPH behaviour for one button key ("f1".."n9"):
// a button not bound in the active template does nothing; a button with
// template text emits that text ({{id}} references resolved first);
// otherwise it acts as a scratch buffer — first press copies the current
// selection into it, later presses emit the stored value.
func (s *State) CopyPaste(key string) {
	t := s.Current()
	b, ok := t.ByKey(key)
	if !ok {
		return
	}

	if b.Text != "" {
		s.emit(s.resolveText(t, b.Text), b.Mode)
		return
	}

	if v, ok := s.scratch[key]; ok {
		s.emit(v, b.Mode)
		return
	}

	if v := copySelection(); v != "" {
		s.scratch[key] = v
	}
}

// Flush clears a scratch buffer (only meaningful for buttons with no template text).
func (s *State) Flush(key string) {
	t := s.Current()
	b, ok := t.ByKey(key)
	if !ok || b.Text != "" {
		return
	}
	delete(s.scratch, key)
}

// resolveText expands {{id}} references in text against the given
// template's ids, recursively. Referenced ids with template text resolve
// to that text (itself resolved); ids without template text resolve to
// their current scratch value (or "" if nothing has been copied into them
// yet). Cycles are already rejected at startup by template.ValidateTemplates;
// the depth cap here is just a defensive backstop.
func (s *State) resolveText(t *template.Template, text string) string {
	return s.resolveTextDepth(t, text, 0)
}

func (s *State) resolveTextDepth(t *template.Template, text string, depth int) string {
	if depth > 20 {
		return text
	}
	return template.ExpandPlaceholders(text, func(id string) (string, bool) {
		b, ok := t.ByID(id)
		if !ok {
			return "", false
		}
		if b.Text != "" {
			return s.resolveTextDepth(t, b.Text, depth+1), true
		}
		return s.scratch[b.Key], true
	})
}

// emit sends text via paste or write mode. modeOverride ("", "paste", or
// "write") comes from the button's own `mode` field and takes precedence
// over the global mode toggled by Numpad * / Numpad /.
func (s *State) emit(text string, modeOverride string) {
	usePaste := s.pasteMode
	switch modeOverride {
	case "paste":
		usePaste = true
	case "write":
		usePaste = false
	}
	if usePaste {
		pasteViaClipboard(text)
	} else {
		_ = desktop.TypeText(text)
	}
}

const clipboardTimeout = 2 * time.Second

// pasteViaClipboard snapshots every clipboard format, temporarily publishes
// text, sends Ctrl+V, then restores the snapshot after the target has had time
// to consume the paste request.
func pasteViaClipboard(text string) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	old, err := desktop.SnapshotClipboard(ctx)
	cancel()
	if err != nil {
		return
	}

	ctx, cancel = context.WithTimeout(context.Background(), clipboardTimeout)
	err = desktop.WriteClipboardText(ctx, text)
	if err == nil {
		err = desktop.SendCtrlV()
	}
	if err == nil {
		timer := time.NewTimer(200 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	cancel()
	restoreClipboard(old)
}

// copySelection sends Ctrl+C and reads back whatever ended up on the
// clipboard, restoring the previous clipboard contents afterwards.
func copySelection() string {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	old, err := desktop.SnapshotClipboard(ctx)
	cancel()
	if err != nil {
		return ""
	}

	ctx, cancel = context.WithTimeout(context.Background(), clipboardTimeout)
	if err := desktop.ClearClipboard(ctx); err != nil {
		cancel()
		return ""
	}
	updates := desktop.WatchClipboardText(ctx)
	if err := desktop.SendCtrlC(); err != nil {
		cancel()
		restoreClipboard(old)
		return ""
	}

	var value string
	select {
	case copied, ok := <-updates:
		if ok {
			value = string(copied)
		}
	case <-ctx.Done():
	}
	cancel()
	restoreClipboard(old)
	return value
}

func restoreClipboard(snapshot desktop.ClipboardSnapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	_ = snapshot.Restore(ctx)
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

// ToggleSuspend flips suspended and notifies. Shared by the Pause hotkey
// and the tray's Suspended checkbox.
func ToggleSuspend(suspended *atomic.Bool) {
	v := !suspended.Load()
	suspended.Store(v)
	if v {
		desktop.Notify("Suspended", "Hotkeys are paused — press <b>Pause</b> to resume")
	} else {
		desktop.Notify("Active", "Hotkeys resumed")
	}
}

// HandleKeyEvent dispatches one hotkey press from the keyboard package onto
// State. quit is called for Ctrl+Pause.
func HandleKeyEvent(state *State, suspended *atomic.Bool, ev keyboard.KeyEvent, quit func()) {
	if ev.Code == keyboard.KeyPause {
		if !ev.Pressed {
			return
		}
		if ev.Ctrl {
			quit()
			return
		}
		ToggleSuspend(suspended)
		return
	}

	if suspended.Load() || !ev.Pressed {
		return
	}

	if slot, ok := keyboard.HotkeyToSlot[ev.Code]; ok {
		if ev.Ctrl {
			state.Flush(slot)
		} else {
			state.CopyPaste(slot)
		}
		return
	}

	switch ev.Code {
	case keyboard.KeyKPPlus:
		state.SwitchTemplate(1)
	case keyboard.KeyKPMinus:
		state.SwitchTemplate(-1)
	case keyboard.KeyKPSlash:
		state.ToggleMode(false)
	case keyboard.KeyKPAsterisk:
		state.ToggleMode(true)
	case keyboard.KeyKP0:
		state.ShowInfo()
	case keyboard.KeyKPDot:
		state.FlushAll()
	}
}
