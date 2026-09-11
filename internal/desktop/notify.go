package desktop

import (
	"html"
	"os/exec"
	"strings"
)

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

// appIcon is a Standard Icon Naming Specification name, so it resolves from
// the desktop's own icon theme (no bundled file needed) on GNOME, KDE, XFCE,
// and friends alike.
const appIcon = "edit-paste"

// Notify shows a desktop notification. body may use the small Pango markup
// subset notification daemons generally support (<b>, <i>, <u>) — use
// EscapeMarkup on any interpolated value that isn't already known to be
// markup-safe, so user-authored text (a template name, pasted content)
// can't break or inject markup.
func Notify(title, body string) {
	if notificationsMuted {
		return
	}
	_ = exec.Command("notify-send", "-a", "copy-paste-helper", "-i", appIcon, title, body).Run()
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
