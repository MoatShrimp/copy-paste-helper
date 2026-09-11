# copy-paste-helper

A Go rewrite of the old AutoHotkey `CPH.ahk` tool for Linux/Wayland and
Windows: turns F1-F12 and the numpad into copy/paste buttons, each either
pasting fixed template text or acting as a scratch clipboard buffer,
configured entirely through YAML templates.

## How it works

On Wayland, the program reads the keyboard directly from `/dev/input/eventX`
(evdev), grabs it exclusively, and re-emits non-hotkey input through a virtual
`/dev/uinput` device. On Windows it uses a low-level keyboard hook, which can
likewise capture and suppress bare function and numpad keys. Numpad bindings
refer to the physical keys regardless of Num Lock on both platforms.

Clipboard access uses [`golang.design/x/clipboard`](https://pkg.go.dev/golang.design/x/clipboard).
On Linux, the same virtual keyboard used for passthrough emits
`Ctrl+C`/`Ctrl+V`; Windows uses native `SendInput`. No separate key-injection
daemon is needed on either platform.
On Wayland, the clipboard library uses the compositor's data-control protocol
when available and otherwise falls back to X11 through XWayland; Windows uses
its native clipboard API. Plain typing ("write" mode) uses Unicode `SendInput`
on Windows. Linux uses `wtype`, whose generated keymap preserves arbitrary text
regardless of the active layout. wtype needs the Wayland virtual-keyboard
protocol — KWin, Sway, and other wlroots compositors support it;
GNOME/Mutter historically doesn't.

## Linux setup

```sh
sudo pacman -S --needed wtype nodejs npm webkit2gtk-4.1 gtk3
go install github.com/wailsapp/wails/v2/cmd/wails@latest   # needs ~/.local/bin (or $(go env GOBIN)) on PATH
sudo usermod -aG input "$USER"
```

Node.js/npm, `wails`, and webkit2gtk/gtk3 are only needed to build the
template editor GUI (`cmd/copy-paste-helper-editor`) — the daemon itself
doesn't link against any of that.

Then **log out and back in** (or `newgrp input`) for the group change to
take effect.

copy-paste-helper needs write access to `/dev/uinput` for its passthrough and
shortcut-injection keyboard. If it isn't writable by your user/group after
joining `input`, add a udev rule such as:

```
# /etc/udev/rules.d/99-uinput.rules
KERNEL=="uinput", GROUP="input", MODE="0660"
```

then `sudo udevadm control --reload && sudo udevadm trigger`.

## Build & run

```sh
# Linux
go build -o copy-paste-helper ./cmd/copy-paste-helper

cd cmd/copy-paste-helper-editor
wails build -tags webkit2_41    # needed if your system only has webkit2gtk-4.1, not 4.0
cd ../..
cp cmd/copy-paste-helper-editor/build/bin/copy-paste-helper-editor .
```

```sh
./copy-paste-helper                 # uses ./templates by default
./copy-paste-helper -templates /path/to/templates
```

The daemon must be able to open `/dev/input/eventX` and `/dev/uinput`, so
run it as your normal user once the group setup above is done (no root
needed). Run it from the repo root (or pass `-templates`) so it finds the
`templates/` directory.

The daemon looks for `copy-paste-helper-editor` right next to its own
executable (falling back to `PATH`) when you use the tray's "Edit
Templates..." item — that's why the `cp` step above puts both binaries in
the same directory.

## Code layout

```
cmd/copy-paste-helper/         the daemon: flag parsing and wiring the pieces below together
cmd/copy-paste-helper-editor/  the Wails + Svelte GUI template editor (a separate binary)
internal/template/             loads/validates/saves/watches the YAML template schema (Button, Template)
internal/keyboard/             evdev device grab + uinput passthrough, emits KeyEvents
internal/engine/               the state machine: active template, scratch buffers, paste mode
internal/desktop/              clipboard, synthetic keystrokes, notifications (shells out to CLI tools)
internal/tray/                 the right-click tray icon and menu
templates/                     example YAML templates
```

Dependencies point one way: `engine` depends on `template`, `keyboard`, and
`desktop` (which don't know about each other or about `engine`), `tray`
depends on `engine`, and `cmd/copy-paste-helper` wires all of them together.
`cmd/copy-paste-helper-editor` depends only on `internal/template` (the same
schema/validation code the daemon uses) — it has no idea the daemon, the
keyboard, or the tray exist.

## Templates

Templates live as `*.yaml` files in the templates directory, loaded in
plain alphabetical filename order — that's also the order Numpad +/- cycles
through. See [templates/1.yaml](templates/1.yaml) and
[templates/2.yaml](templates/2.yaml) for full examples. Format:

```yaml
name: My template
buttons:
  - button: f1 # required: which physical key this binds
    id: email # optional: name other buttons can reference
    template: "you@example.com" # optional: text to paste/type
    tool-tip: "F1: work email" # optional: label for the info popup

  - button: f2
    template: "Reach me at {{email}}" # {{id}} pulls in another button's value
    mode: paste # optional: force "paste" or "write" for this button only

  - button: n1
    tool-tip: "scratch buffer" # no `template` -> this key is a scratch buffer
```

Every field except `button` is optional. Valid button keys are `f1`..`f12`
and `n1`..`n9` (the numpad digit keys — these trigger on the physical key
regardless of Num Lock state). Templates can be any size: a key that isn't
listed in the active template does nothing while that template is active.

For a listed button:

- With `template` set, pressing the key emits that text (after resolving
  any `{{id}}` references).
- Without `template`, the key is a scratch buffer: the first press copies
  the current selection into it, later presses paste it back, and
  Ctrl+that-key clears it.

`{{id}}` references another button's `id` **within the same template** and
is replaced with that button's resolved template text, or — if that button
has no `template` (i.e. it's a scratch buffer) — whatever is currently
stored in its scratch buffer (empty if nothing's been copied into it yet).
References are resolved each time the key is pressed, so a scratch-backed
reference can change between presses.

By default every button pastes or types according to the global mode
(Numpad `*` / Numpad `/`, see below). Set `mode: paste` or `mode: write` on
a button to pin it to one mode regardless of the global toggle — handy for
a button whose text has to come out byte-for-byte (paste) even while you're
generally in write mode, or vice versa.

On startup, every template file is validated together and all problems are
reported at once (not just the first one found): unknown or duplicate
button keys, duplicate ids, `{{id}}` references to an id that doesn't
exist, and circular `{{id}}` references. The program refuses to start until
templates are valid.

## Editing templates with the GUI

Rather than hand-editing YAML, right-click the tray icon and choose **Edit
Templates...** to open `copy-paste-helper-editor` — a small Wails
([wails.io](https://wails.io)) app with a Svelte 5 frontend. It lists every
template in a sidebar; picking one shows its buttons in a form (key,
optional `id`, template text, tool-tip, mode), with:

- **Live validation** as you type (debounced ~350ms), showing the same
  problems `ValidateTemplate` would — you can't save until they're fixed.
- **Live `{{id}}` preview** under any button whose template text references
  another button, so you can see the resolved result without saving.
- **+ New Template** / **+ Add button** / a ✕ to remove a button or delete
  a whole template (with confirmation).

The editor is a separate process from the daemon and only ever touches
files in the templates directory — it has no idea a daemon is even running.
That's what hot reload (below) is for.

### Hot reload

The running daemon watches the templates directory and reloads whenever a
file changes — saving in the editor (or hand-editing a YAML file, or `git
pull`-ing new templates) takes effect immediately, no restart needed:

- A successful reload re-validates everything, fires a "Templates Reloaded"
  notification, and rebuilds the tray's Template submenu to match.
- Your active template stays selected (matched by file path) with its
  scratch buffers intact, unless that exact file was deleted/renamed, in
  which case selection falls back to the first template.
- If the new state of the directory fails validation, the daemon logs the
  problem, shows a "Templates: Reload Failed" notification, and keeps
  running on the last good set of templates — a bad edit never crashes it
  or leaves it stuck mid-reload.

## Keys

| Key                       | Action                                                                 |
| ------------------------- | ---------------------------------------------------------------------- |
| F1-F12, numpad 1-9        | Copy/paste that button (if bound in the active template)               |
| Ctrl + (one of the above) | Flush that button's scratch buffer                                     |
| Numpad `Del` (`.`)        | Flush all scratch buffers                                              |
| Numpad `+` / `-`          | Next / previous template                                               |
| Numpad `/`                | Switch to "write" mode (types text out directly)                       |
| Numpad `*`                | Switch to "paste" mode (via clipboard, `Ctrl+V`)                       |
| Numpad `Ins` (`0`)        | Show a notification with all slot labels                               |
| Pause                     | Suspend/resume all hotkeys (keyboard behaves normally while suspended) |
| Ctrl + Pause              | Quit                                                                   |

Notifications are sent directly through the tray library's native backend
(D-Bus on Linux and Win32 on Windows). Linux notifications use a small amount
of markup for readability; Windows notifications are converted to plain text.
User-authored values are escaped before interpolation.

## Tray icon

A right-click tray icon (StatusNotifierItem D-Bus on Linux, native Win32 on
Windows) mirrors and controls the same state as the hotkeys:

- **Edit Templates...** — launches the template editor GUI (see above).
- **Template** — submenu listing every loaded template; click one to switch
  to it directly (same effect as Numpad +/-, but not limited to stepping
  one at a time).
- **Mode** — Write / Paste, same as Numpad `/` and `*`.
- **Suspended** — same as pressing Pause.
- **Quit** — same as Ctrl+Pause.

The checkboxes stay in sync no matter whether a change comes from the menu
or from the keyboard. The icon itself switches between a clipboard (active)
and a clipboard with a slash through it (suspended) — [clipboard](https://tabler.io/icons/icon/clipboard)
and [clipboard-off](https://tabler.io/icons/icon/clipboard-off) from
[Tabler Icons](https://tabler.io/icons) (MIT licensed), rendered black with
a white halo so they stay legible on both light and dark panels.

Changing something from the menu doesn't also pop up a notification — the
menu's own checkmarks already show the result, so a toast for the same change
would just be noise. Notifications still fire for changes made from the
keyboard, since those have no other visual feedback.
