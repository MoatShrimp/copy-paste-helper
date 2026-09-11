package template

import (
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchResult is sent on Watch's channel whenever the templates directory
// changes: either a freshly discovered and validated template list, or the
// error that prevented that (in which case the caller should keep running
// on whatever templates it already has).
type WatchResult struct {
	Templates []*Template
	Err       error
}

// Watch watches dir for .yaml/.yml changes (including creates, removes, and
// editor save patterns like write-to-temp-then-rename) and sends a
// re-discovered, re-validated template list on the returned channel after
// each burst of changes settles. Call the returned stop function to stop
// watching and release the underlying inotify watch; it's safe to call more
// than once.
func Watch(dir string) (<-chan WatchResult, func() error, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, nil, err
	}

	results := make(chan WatchResult)
	go func() {
		defer close(results)

		// Coalesce a burst of events (an editor's save is often several:
		// write a temp file, rename it over the target, etc.) into one
		// reload, fired debounce after the last event in the burst.
		const debounce = 300 * time.Millisecond
		var timer *time.Timer
		var timerC <-chan time.Time

		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				if timer == nil {
					timer = time.NewTimer(debounce)
				} else {
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(debounce)
				}
				timerC = timer.C
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			case <-timerC:
				timer, timerC = nil, nil
				templates, err := DiscoverTemplates(dir)
				if err == nil {
					if verr := ValidateTemplates(templates); verr != nil {
						err = verr
					}
				}
				results <- WatchResult{Templates: templates, Err: err}
			}
		}
	}()

	return results, w.Close, nil
}
