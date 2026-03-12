// Package watcher defines the FileWatcher interface for monitoring file changes.
// v1 ships with NoOpWatcher; v2 will replace it with an fsnotify-based implementation.
package watcher

// FileWatcher watches a file for modifications and invokes OnChange.
type FileWatcher interface {
	// Start begins watching. Returns an error if the watcher cannot be initialised.
	Start() error
	// Stop terminates watching and releases resources.
	Stop()
	// OnChange registers the callback invoked when the watched file changes.
	OnChange(fn func())
}

// NoOpWatcher is a FileWatcher that does nothing. Used in v1.
type NoOpWatcher struct{}

func (n *NoOpWatcher) Start() error  { return nil }
func (n *NoOpWatcher) Stop()         {}
func (n *NoOpWatcher) OnChange(_ func()) {}
