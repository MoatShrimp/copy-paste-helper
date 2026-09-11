package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"copy-paste-helper/internal/template"
)

// App is the Wails-bound backend for the template editor UI. Every
// exported method here becomes callable from the Svelte frontend.
type App struct {
	ctx context.Context
	dir string
}

func NewApp(templatesDir string) *App {
	return &App{dir: templatesDir}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ButtonKeys returns every valid physical button key, in display order —
// the single source of truth the frontend uses to populate the "button"
// dropdown, so it never has to hard-code f1..f12/n1..n9 itself.
func (a *App) ButtonKeys() []string {
	return template.SlotOrder
}

// ListTemplates loads every template currently in the templates directory.
// It does not validate them — call ValidateTemplate per template if the UI
// wants to flag pre-existing problems up front.
func (a *App) ListTemplates() ([]*template.Template, error) {
	return template.DiscoverTemplates(a.dir)
}

// errorStrings converts a slice of errors (as returned by
// template.ValidateTemplate) into plain strings for the frontend.
func errorStrings(errs []error) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	return out
}

// ValidateTemplate checks one template in isolation (it does not need to
// have been saved) and returns a list of human-readable problems — empty
// if it's valid.
func (a *App) ValidateTemplate(t *template.Template) []string {
	return errorStrings(template.ValidateTemplate(t))
}

// PreviewText resolves {{id}} placeholders in text against t's in-memory
// buttons (which need not be saved yet), for a live preview as the user
// types.
func (a *App) PreviewText(t *template.Template, text string) string {
	return template.PreviewText(t, text)
}

// filenameFor turns a user-provided template name into a safe *.yaml
// filename, e.g. "Work Email" -> "work-email.yaml".
func filenameFor(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "template"
	}
	return slug + ".yaml"
}

// NewTemplateFilename previews the filename SaveTemplate would create for
// name, so the UI can show it before saving.
func (a *App) NewTemplateFilename(name string) string {
	return filenameFor(name)
}

// SaveResult is SaveTemplate's outcome: either Template is populated (with
// Path set) on success, or Errors lists why it wasn't saved. Bundled into
// one struct because Wails' TS binding generator only understands methods
// returning (value) or (value, error), not three separate values.
type SaveResult struct {
	Template *template.Template `json:"template,omitempty"`
	Errors   []string           `json:"errors,omitempty"`
}

// SaveTemplate validates and writes a template to disk. If t.Path is
// empty, a new file is created (named from t.Name, de-duplicated if
// needed); otherwise the existing file at t.Path is overwritten. The
// returned error is only for unexpected I/O failures — a validation
// failure comes back as a normal result with Errors set.
func (a *App) SaveTemplate(t *template.Template) (*SaveResult, error) {
	if errs := template.ValidateTemplate(t); len(errs) != 0 {
		return &SaveResult{Errors: errorStrings(errs)}, nil
	}

	if t.Path == "" {
		base := filenameFor(t.Name)
		path := filepath.Join(a.dir, base)
		for i := 2; ; i++ {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				break
			}
			ext := filepath.Ext(base)
			path = filepath.Join(a.dir, fmt.Sprintf("%s-%d%s", strings.TrimSuffix(base, ext), i, ext))
		}
		t.Path = path
	}

	if err := template.SaveTemplate(t); err != nil {
		return nil, err
	}
	return &SaveResult{Template: t}, nil
}

// DeleteTemplate removes a template file from disk.
func (a *App) DeleteTemplate(path string) error {
	return os.Remove(path)
}
