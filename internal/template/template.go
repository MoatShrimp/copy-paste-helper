// Package template loads, validates, and resolves the YAML files that
// configure copy-paste-helper's buttons — the "template" schema. It knows
// nothing about the keyboard or how text actually gets pasted/typed.
package template

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Button is one entry in a template's button list, binding a physical key
// to either fixed template text or (when Text is empty) a scratch buffer.
// JSON tags exist so this same struct can be sent to/from the Wails editor
// UI without a parallel DTO type.
type Button struct {
	Key     string `yaml:"button" json:"button"`
	ID      string `yaml:"id,omitempty" json:"id,omitempty"`
	Text    string `yaml:"template,omitempty" json:"template,omitempty"`
	ToolTip string `yaml:"tool-tip,omitempty" json:"toolTip,omitempty"`
	Mode    string `yaml:"mode,omitempty" json:"mode,omitempty"` // "", "paste", or "write" — overrides the global mode for this button
}

type Template struct {
	Name    string   `yaml:"name" json:"name"`
	Buttons []Button `yaml:"buttons" json:"buttons"`

	Path string `yaml:"-" json:"path"`

	byKey map[string]*Button `yaml:"-" json:"-"`
	byID  map[string]*Button `yaml:"-" json:"-"`
}

// ByKey returns the button bound to a physical key ("f1".."n9"), if any.
// Only populated after ValidateTemplates has run.
func (t *Template) ByKey(key string) (*Button, bool) {
	b, ok := t.byKey[key]
	return b, ok
}

// ByID returns the button with the given `id`, if any. Only populated
// after ValidateTemplates has run.
func (t *Template) ByID(id string) (*Button, bool) {
	b, ok := t.byID[id]
	return b, ok
}

// SlotOrder lists every valid button key, in display order. This is the
// only hard-coded piece: which physical keys exist. Everything else about
// how a key behaves comes from the active template.
var SlotOrder = []string{
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
	"n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9",
}

var validButtonKeys = func() map[string]bool {
	m := make(map[string]bool, len(SlotOrder))
	for _, k := range SlotOrder {
		m[k] = true
	}
	return m
}()

// placeholderRe matches {{id}} references inside a button's template text.
var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

func referencedIDs(text string) []string {
	matches := placeholderRe.FindAllStringSubmatch(text, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m[1])
	}
	return ids
}

// ExpandPlaceholders replaces every {{id}} reference in text using resolve,
// which maps an id to its value (and whether it exists). An id resolve
// reports as missing is left untouched. This only does one substitution
// pass; a resolve func that itself needs recursive expansion (as
// engine.State does, for a scratch-backed reference) must do that itself.
func ExpandPlaceholders(text string, resolve func(id string) (value string, ok bool)) string {
	return placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		id := placeholderRe.FindStringSubmatch(m)[1]
		v, ok := resolve(id)
		if !ok {
			return m
		}
		return v
	})
}

func LoadTemplate(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if t.Name == "" {
		t.Name = filepath.Base(path)
	}
	t.Path = path
	return &t, nil
}

// SaveTemplate writes t to t.Path as YAML (t.Path must already be set),
// via a write-then-rename so a watcher never observes a half-written file.
func SaveTemplate(t *Template) error {
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	tmp := t.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.Path)
}

// DiscoverTemplates finds *.yaml/*.yml files in dir and loads them in plain
// alphabetical filename order; that order is what Numpad +/- cycles through.
func DiscoverTemplates(dir string) ([]*Template, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)

	var templates []*Template
	for _, p := range paths {
		t, err := LoadTemplate(p)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, nil
}

// ValidateTemplate checks one template in isolation for structural problems
// (unknown or duplicate button keys, duplicate ids, invalid mode) and, once
// its lookup tables are built, {{id}} references that point nowhere or form
// a cycle. It also populates t's byKey/byID maps used at runtime and by
// PreviewText. Since {{id}} only ever refers within the same template,
// templates never need each other's data to validate. Returns nil if t is
// valid.
func ValidateTemplate(t *Template) []error {
	var errs []error

	t.byKey = map[string]*Button{}
	t.byID = map[string]*Button{}
	for i := range t.Buttons {
		b := &t.Buttons[i]
		if b.Key == "" {
			errs = append(errs, fmt.Errorf("button entry #%d is missing the required 'button' field", i+1))
			continue
		}
		if !validButtonKeys[b.Key] {
			errs = append(errs, fmt.Errorf("unknown button %q (valid: %s)", b.Key, strings.Join(SlotOrder, ", ")))
			continue
		}
		if _, dup := t.byKey[b.Key]; dup {
			errs = append(errs, fmt.Errorf("button %q is defined more than once", b.Key))
			continue
		}
		if b.Mode != "" && b.Mode != "paste" && b.Mode != "write" {
			errs = append(errs, fmt.Errorf("button %q has invalid mode %q (valid: paste, write)", b.Key, b.Mode))
			continue
		}
		t.byKey[b.Key] = b

		if b.ID != "" {
			if _, dup := t.byID[b.ID]; dup {
				errs = append(errs, fmt.Errorf("id %q is defined more than once", b.ID))
				continue
			}
			t.byID[b.ID] = b
		}
	}
	if len(errs) > 0 {
		return errs
	}

	for i := range t.Buttons {
		b := &t.Buttons[i]
		for _, id := range referencedIDs(b.Text) {
			if _, ok := t.byID[id]; !ok {
				errs = append(errs, fmt.Errorf("button %q template references unknown id %q", b.Key, id))
			}
		}
	}
	if cycle := findIDCycle(t); cycle != "" {
		errs = append(errs, fmt.Errorf("circular id reference: %s", cycle))
	}
	return errs
}

// ValidateTemplates validates every template (see ValidateTemplate) and
// reports all problems found across all of them together, each prefixed
// with its file path.
func ValidateTemplates(templates []*Template) error {
	var errs []error
	for _, t := range templates {
		for _, e := range ValidateTemplate(t) {
			errs = append(errs, fmt.Errorf("%s: %w", t.Path, e))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// PreviewText expands {{id}} references in text against t's own buttons,
// without requiring t to have been validated first — used by the editor to
// show a live preview while the user is still typing. An id with no
// template text (i.e. a scratch button) previews as ""; an unknown id or a
// cycle is left as "" too (ValidateTemplate is what reports those as
// actual errors).
func PreviewText(t *Template, text string) string {
	lookup := make(map[string]*Button, len(t.Buttons))
	for i := range t.Buttons {
		if b := &t.Buttons[i]; b.ID != "" {
			lookup[b.ID] = b
		}
	}

	var resolve func(text string, depth int) string
	resolve = func(text string, depth int) string {
		if depth > 20 {
			return ""
		}
		return ExpandPlaceholders(text, func(id string) (string, bool) {
			b, ok := lookup[id]
			if !ok {
				return "", false
			}
			return resolve(b.Text, depth+1), true
		})
	}
	return resolve(text, 0)
}

// findIDCycle reports the first cycle found among {{id}} references within
// a template, formatted as "a -> b -> a", or "" if there is none.
func findIDCycle(t *Template) string {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := map[string]int{}
	var path []string
	var cycle string

	var dfs func(id string)
	dfs = func(id string) {
		state[id] = visiting
		path = append(path, id)
		b := t.byID[id]
		for _, ref := range referencedIDs(b.Text) {
			if cycle != "" {
				return
			}
			if _, ok := t.byID[ref]; !ok {
				continue // unknown ids are reported separately
			}
			switch state[ref] {
			case visiting:
				idx := 0
				for i, p := range path {
					if p == ref {
						idx = i
						break
					}
				}
				cycle = strings.Join(path[idx:], " -> ") + " -> " + ref
				return
			case unvisited:
				dfs(ref)
			}
		}
		path = path[:len(path)-1]
		state[id] = done
	}

	// Sort ids for deterministic error messages.
	ids := make([]string, 0, len(t.byID))
	for id := range t.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if state[id] == unvisited {
			dfs(id)
			if cycle != "" {
				return cycle
			}
		}
	}
	return ""
}
