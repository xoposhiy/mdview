package watcher

import (
	"time"

	"github.com/fsnotify/fsnotify"
)

// FsnotifyWatcher watches a file using fsnotify with debouncing.
// Handles editor rename-on-save patterns (vim, VS Code, etc.).
type FsnotifyWatcher struct {
	path     string
	onChange func()
	fw       *fsnotify.Watcher
	done     chan struct{}
}

func NewFsnotifyWatcher(path string) *FsnotifyWatcher {
	return &FsnotifyWatcher{path: path}
}

func (w *FsnotifyWatcher) OnChange(fn func()) { w.onChange = fn }

func (w *FsnotifyWatcher) Start() error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := fw.Add(w.path); err != nil {
		fw.Close()
		return err
	}
	w.fw = fw
	w.done = make(chan struct{})
	go w.loop()
	return nil
}

func (w *FsnotifyWatcher) Stop() {
	if w.done != nil {
		close(w.done)
	}
	if w.fw != nil {
		w.fw.Close()
	}
}

func (w *FsnotifyWatcher) loop() {
	var debounce *time.Timer
	fire := func() {
		if w.onChange != nil {
			w.onChange()
		}
	}
	schedule := func() {
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.AfterFunc(80*time.Millisecond, fire)
	}

	for {
		select {
		case <-w.done:
			if debounce != nil {
				debounce.Stop()
			}
			return
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				schedule()
			}
			if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
				// Editor may have replaced the file via rename; re-add and reload.
				w.fw.Add(w.path)
				schedule()
			}
		case _, ok := <-w.fw.Errors:
			if !ok {
				return
			}
		}
	}
}
