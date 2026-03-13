package main

import (
	"fmt"
	"os"

	"mdview/internal/config"
	"mdview/internal/renderer"
	"mdview/internal/viewer"
	"mdview/internal/watcher"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mdview <file.md>")
		os.Exit(1)
	}

	path := os.Args[1]

	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Load()
	dark := cfg.Theme == "dark"

	html, err := renderer.RenderWithTheme(content, dark, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rendering: %v\n", err)
		os.Exit(1)
	}

	v := viewer.New(path, html, dark, renderer.RenderWithTheme)


	w := watcher.NewFsnotifyWatcher(path)
	w.OnChange(func() { v.Reload() })
	if err := w.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: file watching unavailable: %v\n", err)
	}
	defer w.Stop()

	v.Run()
}
