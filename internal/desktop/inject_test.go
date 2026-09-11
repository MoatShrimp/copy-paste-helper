package desktop

import (
	"slices"
	"testing"
)

func TestClipboardShortcutsUseConfiguredKeySender(t *testing.T) {
	defer SetKeySender(nil)

	var calls [][]int
	SetKeySender(func(codes ...int) error {
		calls = append(calls, slices.Clone(codes))
		return nil
	})

	if err := SendCtrlC(); err != nil {
		t.Fatalf("SendCtrlC: %v", err)
	}
	if err := SendCtrlV(); err != nil {
		t.Fatalf("SendCtrlV: %v", err)
	}

	want := [][]int{{keyLeftCtrl, keyC}, {keyLeftCtrl, keyV}}
	if len(calls) != len(want) {
		t.Fatalf("key sender call count = %d, want %d", len(calls), len(want))
	}
	if !slices.Equal(calls[0], want[0]) || !slices.Equal(calls[1], want[1]) {
		t.Fatalf("key sender calls = %v, want %v", calls, want)
	}
}
