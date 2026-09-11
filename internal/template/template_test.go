package template

import "testing"

func TestValidateTemplateStandalone(t *testing.T) {
	tpl := &Template{
		Name: "T",
		Buttons: []Button{
			{Key: "f1", ID: "a", Text: "hello {{b}}"},
			{Key: "f2", ID: "b", Text: "world"},
			{Key: "n1"}, // scratch button, no id
		},
	}
	if errs := ValidateTemplate(tpl); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	got := PreviewText(tpl, tpl.Buttons[0].Text)
	if got != "hello world" {
		t.Fatalf("PreviewText = %q, want %q", got, "hello world")
	}
}

func TestValidateTemplateCatchesProblems(t *testing.T) {
	cases := []struct {
		name string
		tpl  *Template
	}{
		{"unknown button", &Template{Buttons: []Button{{Key: "q1"}}}},
		{"duplicate button", &Template{Buttons: []Button{{Key: "f1"}, {Key: "f1"}}}},
		{"duplicate id", &Template{Buttons: []Button{{Key: "f1", ID: "x"}, {Key: "f2", ID: "x"}}}},
		{"unknown ref", &Template{Buttons: []Button{{Key: "f1", Text: "{{nope}}"}}}},
		{"cycle", &Template{Buttons: []Button{
			{Key: "f1", ID: "a", Text: "{{b}}"},
			{Key: "f2", ID: "b", Text: "{{a}}"},
		}}},
		{"bad mode", &Template{Buttons: []Button{{Key: "f1", Mode: "explode"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errs := ValidateTemplate(c.tpl); len(errs) == 0 {
				t.Fatalf("expected an error for %s, got none", c.name)
			}
		})
	}
}

func TestPreviewTextScratchButtonIsEmpty(t *testing.T) {
	tpl := &Template{Buttons: []Button{
		{Key: "f1", Text: "value: {{s}}"},
		{Key: "n1", ID: "s"}, // scratch, no Text
	}}
	if errs := ValidateTemplate(tpl); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	got := PreviewText(tpl, tpl.Buttons[0].Text)
	if got != "value: " {
		t.Fatalf("PreviewText = %q, want %q", got, "value: ")
	}
}
