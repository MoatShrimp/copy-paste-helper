// Package desktop is copy-paste-helper's only point of contact with the
// outside desktop environment: the clipboard, synthetic keystrokes, and
// notifications. Clipboard and notification access use Go libraries; text
// entry uses the native Windows API or wtype on Linux.
package desktop

import (
	"context"
	"errors"
	"fmt"

	"golang.design/x/clipboard"
)

// ClipboardSnapshot contains every representation currently advertised by the
// clipboard. Keeping the items private prevents callers from depending on the
// underlying clipboard package's types.
type ClipboardSnapshot struct {
	items []clipboard.Item
}

// InitClipboard initializes access to the system clipboard. It must be called
// once before any other clipboard operation.
func InitClipboard() error {
	return clipboard.Init()
}

// SnapshotClipboard reads all available clipboard formats so a temporary text
// paste does not discard an existing image, file list, or custom MIME value.
func SnapshotClipboard(ctx context.Context) (ClipboardSnapshot, error) {
	formats, err := clipboard.Formats(ctx)
	if err != nil {
		return ClipboardSnapshot{}, fmt.Errorf("list clipboard formats: %w", err)
	}

	snapshot := ClipboardSnapshot{items: make([]clipboard.Item, 0, len(formats))}
	for _, format := range formats {
		data, err := clipboard.Read(ctx, format)
		if errors.Is(err, clipboard.ErrNoData) {
			continue
		}
		if err != nil {
			return ClipboardSnapshot{}, fmt.Errorf("read clipboard format %q: %w", format.MIME(), err)
		}
		snapshot.items = append(snapshot.items, clipboard.Item{Format: format, Bytes: data})
	}
	return snapshot, nil
}

// Restore replaces the clipboard with all formats captured in the snapshot.
func (snapshot ClipboardSnapshot) Restore(ctx context.Context) error {
	if len(snapshot.items) == 0 {
		return ClearClipboard(ctx)
	}
	opts := make([]clipboard.Option, 0, len(snapshot.items))
	for _, item := range snapshot.items {
		opts = append(opts, item)
	}
	_, err := clipboard.WriteAll(ctx, opts...)
	return err
}

// WriteClipboardText publishes text until another write replaces it. We avoid
// clipboard.Loops here because clipboard managers may consume a serve before
// the application receiving Ctrl+V does.
func WriteClipboardText(ctx context.Context, text string) error {
	_, err := clipboard.Write(ctx, clipboard.FmtText, []byte(text))
	return err
}

func ClearClipboard(ctx context.Context) error {
	_, err := clipboard.Write(ctx, clipboard.FmtText, nil)
	return err
}

// WatchClipboardText reports non-empty text whenever the clipboard changes.
func WatchClipboardText(ctx context.Context) <-chan []byte {
	updates := clipboard.Watch(ctx, clipboard.FmtText)
	text := make(chan []byte, 1)
	go func() {
		defer close(text)
		for update := range updates {
			if len(update.Bytes) == 0 {
				continue
			}
			select {
			case text <- update.Bytes:
			case <-ctx.Done():
				return
			}
		}
	}()
	return text
}
