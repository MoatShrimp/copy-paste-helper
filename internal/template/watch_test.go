package template

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("1.yaml", "name: One\nbuttons:\n  - button: f1\n    template: hello\n")

	results, stop, err := Watch(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	write("2.yaml", "name: Two\nbuttons:\n  - button: f2\n    template: world\n")

	select {
	case r := <-results:
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		if len(r.Templates) != 2 {
			t.Fatalf("expected 2 templates after reload, got %d", len(r.Templates))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reload after adding a template")
	}

	// A change that breaks validation should still report a result (with Err set).
	write("3.yaml", "name: Bad\nbuttons:\n  - button: not-a-real-key\n")

	select {
	case r := <-results:
		if r.Err == nil {
			t.Fatal("expected an error for the invalid template, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reload after adding an invalid template")
	}
}
