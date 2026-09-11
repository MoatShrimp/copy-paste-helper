package engine

import (
	"testing"

	"copy-paste-helper/internal/template"
)

func TestResolveTextAndScratch(t *testing.T) {
	templates, err := template.DiscoverTemplates("../../templates")
	if err != nil {
		t.Fatal(err)
	}
	if err := template.ValidateTemplates(templates); err != nil {
		t.Fatal(err)
	}

	s := NewState(templates)

	tpl := s.Current()
	f2, ok := tpl.ByKey("f2")
	if !ok {
		t.Fatal("expected f2 to be bound in the example template")
	}
	got := s.resolveText(tpl, f2.Text)
	want := "Reach me at you@example.com.\n\nBest regards,\nMathias\n"
	if got != want {
		t.Fatalf("resolveText f2 = %q, want %q", got, want)
	}

	n2, ok := tpl.ByKey("n2")
	if !ok {
		t.Fatal("expected n2 to be bound in the example template")
	}
	if n2.Text != "" {
		t.Fatalf("n2 expected to be a scratch button (no template text)")
	}
	if _, ok := s.scratch["n2"]; ok {
		t.Fatalf("n2 scratch should start empty")
	}
	s.scratch["n2"] = "captured"
	if got := s.scratch["n2"]; got != "captured" {
		t.Fatalf("scratch n2 = %q", got)
	}

	s.Flush("n2")
	if _, ok := s.scratch["n2"]; ok {
		t.Fatalf("flush should have cleared n2")
	}
}
