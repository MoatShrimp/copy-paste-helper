// Command copy-paste-helper turns F1-F12 and the numpad into copy/paste
// buttons, configured via YAML templates, with a right-click tray icon.
// See the package doc comments under internal/ for how the pieces fit
// together: template (config), keyboard (evdev), engine (state machine),
// desktop (clipboard/injection/notifications), tray (systray UI).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"copy-paste-helper/internal/desktop"
	"copy-paste-helper/internal/engine"
	"copy-paste-helper/internal/keyboard"
	"copy-paste-helper/internal/template"
	"copy-paste-helper/internal/tray"
)

func main() {
	templatesDir := flag.String("templates", "templates", "directory containing template YAML files")
	flag.Parse()

	templates, err := template.DiscoverTemplates(*templatesDir)
	if err != nil {
		log.Fatalf("loading templates: %v", err)
	}
	if len(templates) == 0 {
		log.Fatalf("no templates found in %s (expected *.yaml files)", *templatesDir)
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

	done := make(chan struct{})
	var quitOnce sync.Once
	quit := func() { quitOnce.Do(func() { close(done) }) }

	commands := make(chan func(), 8)
	tr := tray.New(state, &suspended, commands, quit)
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
		}
	}
}
