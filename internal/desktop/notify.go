package desktop

import (
	"html"
	"runtime"
	"strings"
	"sync"
)

var (
	notifierMu sync.RWMutex
	notifier   func(title, body string)
)

// SetNotifier installs the application's desktop-notification backend. Passing
// nil disables notifications. The tray package supplies the production
// implementation once its D-Bus connection has been created.
func SetNotifier(fn func(title, body string)) {
	notifierMu.Lock()
	notifier = fn
	notifierMu.Unlock()
}

// notificationsMuted is set while a command from the tray menu is being
// applied: the menu (its checkmarks, its icon) already shows the result, so
// a popup notification for the same change would just be noise. It's only
// ever touched from the main goroutine's event loop, so it needs no locking.
var notificationsMuted bool

// SetNotificationsMuted suppresses (or restores) Notify/NotifyLines. Callers
// should always restore it to false once the muted action is done.
func SetNotificationsMuted(muted bool) {
	notificationsMuted = muted
}

// Notify shows a desktop notification. body may use the small Pango markup
// subset notification daemons generally support (<b>, <i>, <u>) — use
// EscapeMarkup on any interpolated value that isn't already known to be
// markup-safe, so user-authored text (a template name, pasted content)
// can't break or inject markup.
func Notify(title, body string) {
	if notificationsMuted {
		return
	}
	notifierMu.RLock()
	fn := notifier
	notifierMu.RUnlock()
	if fn != nil {
		fn(title, notificationBody(body))
	}
}

func notificationBody(body string) string {
	if runtime.GOOS != "windows" {
		return body
	}
	withoutMarkup := strings.NewReplacer(
		"<b>", "", "</b>", "",
		"<i>", "", "</i>", "",
		"<u>", "", "</u>", "",
	).Replace(body)
	return html.UnescapeString(withoutMarkup)
}

func NotifyLines(title string, lines []string) {
	Notify(title, strings.Join(lines, "\n"))
}

// EscapeMarkup escapes a plain-text value for safe interpolation into a
// Notify/NotifyLines body that otherwise contains Pango markup, so text the
// user wrote (a template name, pasted content) can't be misread as markup.
func EscapeMarkup(s string) string {
	return html.EscapeString(s)
}
