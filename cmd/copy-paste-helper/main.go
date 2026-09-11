// Command copy-paste-helper turns F1-F12 and the numpad into copy/paste
// buttons, configured via YAML templates, with a right-click tray icon.
// See the package doc comments under internal/ for how the pieces fit
// together: template (config), keyboard (evdev), engine (state machine),
// desktop (clipboard/injection/notifications), tray (systray UI).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"copy-paste-helper/internal/desktop"
	"copy-paste-helper/internal/engine"
	"copy-paste-helper/internal/keyboard"
	"copy-paste-helper/internal/template"
	"copy-paste-helper/internal/tray"
)

func main() {
	templatesDirFlag := flag.String("templates", "templates", "directory containing template YAML files")
	flag.Parse()

	templatesDir, err := filepath.Abs(*templatesDirFlag)
	if err != nil {
		log.Fatalf("resolving templates directory: %v", err)
	}

	templates, err := template.DiscoverTemplates(templatesDir)
	if err != nil {
		log.Fatalf("loading templates: %v", err)
	}
	if len(templates) == 0 {
		log.Fatalf("no templates found in %s (expected *.yaml files)", templatesDir)
	}
	if err := template.ValidateTemplates(templates); err != nil {
		log.Fatalf("template validation failed:\n%v", err)
	}

	devices, err := keyboard.FindDevices()
	if err != nil {
		log.Fatalf("%v", err)
	}

	var suspended atomic.Bool

	events := make(chan keyboard.KeyEvent, 32)
	var listeners []*keyboard.Listener
	for _, dev := range devices {
		kl, err := keyboard.Open(dev, &suspended)
		if err != nil {
			log.Printf("skipping %s: %v", dev, err)
			continue
		}
		listeners = append(listeners, kl)
		go func(kl *keyboard.Listener) {
			for ev := range kl.Events {
				events <- ev
			}
		}(kl)
	}
	if len(listeners) == 0 {
		log.Fatalf("could not grab any keyboard device (are you in the 'input' group and can you access /dev/uinput?)")
	}

	state := engine.NewState(templates)

	reloads, stopWatch, err := template.Watch(templatesDir)
	if err != nil {
		log.Printf("watching %s for changes: %v (edits won't apply until restart)", templatesDir, err)
	} else {
		defer stopWatch()
	}

	done := make(chan struct{})
	var quitOnce sync.Once
	quit := func() { quitOnce.Do(func() { close(done) }) }

	commands := make(chan func(), 8)
	editTemplates := func() { launchEditor(templatesDir) }
	tr := tray.New(state, &suspended, commands, editTemplates, quit)
	defer tr.Remove()
	go func() {
		if err := tr.Run(); err != nil {
			log.Printf("tray: %v", err)
		}
	}()

	defer func() {
		for _, kl := range listeners {
			kl.Close()
		}
	}()

	desktop.Notify("copy-paste-helper is running", fmt.Sprintf("Active template: <b>%s</b>", desktop.EscapeMarkup(state.Current().Name)))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		quit()
	}()

	for {
		select {
		case <-done:
			return
		case ev, ok := <-events:
			if !ok {
				log.Fatalf("keyboard device closed unexpectedly")
			}
			engine.HandleKeyEvent(state, &suspended, ev, quit)
			tr.Sync(state, suspended.Load())
		case cmd := <-commands:
			desktop.SetNotificationsMuted(true)
			cmd()
			desktop.SetNotificationsMuted(false)
			tr.Sync(state, suspended.Load())
		case r, ok := <-reloads:
			if !ok {
				reloads = nil // watcher died; stop selecting on it, keep running on what we have
				continue
			}
			if r.Err != nil {
				log.Printf("template reload failed: %v", r.Err)
				desktop.Notify("Templates: Reload Failed", desktop.EscapeMarkup(r.Err.Error()))
				continue
			}
			state.ReplaceTemplates(r.Templates)
			tr.Rebuild(state, &suspended, commands, editTemplates, quit)
			desktop.Notify("Templates Reloaded", fmt.Sprintf("Active template: <b>%s</b>", desktop.EscapeMarkup(state.Current().Name)))
		}
	}
}

// launchEditor starts the GUI template editor as a separate process,
// pointed at the same templates directory this daemon is using. It looks
// for a "copy-paste-helper-editor" binary next to the running executable
// first, falling back to PATH.
func launchEditor(templatesDir string) {
	editorPath := "copy-paste-helper-editor"
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), "copy-paste-helper-editor")
		if _, err := os.Stat(sibling); err == nil {
			editorPath = sibling
		}
	}

	cmd := exec.Command(editorPath, "-templates", templatesDir)
	stderr := &limitedBuffer{limit: 4096}
	cmd.Stderr = stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		log.Printf("launching editor: %v", err)
		desktop.Notify("Edit Templates", fmt.Sprintf("Could not launch the editor: %s", desktop.EscapeMarkup(err.Error())))
		return
	}

	// A GUI app that's actually working stays open; one that's crashing
	// (a missing runtime dependency, a bad build) exits almost instantly.
	// Surface that instead of silently doing nothing, which is what a
	// cmd.Wait()-and-discard would otherwise look like from the tray.
	go func() {
		waitErr := cmd.Wait()
		if waitErr == nil || time.Since(start) > 5*time.Second {
			return
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		log.Printf("editor exited immediately: %v\n%s", waitErr, msg)
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		desktop.Notify("Edit Templates", fmt.Sprintf("The editor closed immediately: %s", desktop.EscapeMarkup(msg)))
	}()
}

// limitedBuffer captures up to limit bytes written to it and silently drops
// the rest, so capturing a crashing child's stderr can't grow unbounded if
// it instead runs fine for hours.
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if room > len(p) {
			room = len(p)
		}
		b.buf.Write(p[:room])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
