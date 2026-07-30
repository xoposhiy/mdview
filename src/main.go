package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mdview/internal/config"
	"mdview/internal/renderer"
	"mdview/internal/viewer"
	"mdview/internal/watcher"
)

func main() {
	// Handle --export=<path> flag: render markdown to HTML and exit without GUI.
	exportPath := ""
	for i, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--export=") {
			exportPath = strings.TrimPrefix(arg, "--export=")
			os.Args = append(os.Args[:i+1], os.Args[i+2:]...)
			break
		}
	}

	if exportPath != "" {
		if len(os.Args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: mdview --export=<output.html> <file.md>")
			os.Exit(1)
		}
		mdPath := os.Args[1]
		content, err := os.ReadFile(mdPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
			os.Exit(1)
		}
		html, err := renderer.RenderWithTheme(content, config.Default(), 0, filepath.Dir(mdPath))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error rendering: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(exportPath, []byte(html), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

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

	html, err := renderer.RenderWithTheme(content, cfg, 0, filepath.Dir(path))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rendering: %v\n", err)
		os.Exit(1)
	}

	v := viewer.New(path, html, cfg, renderer.RenderWithTheme)


	w := watcher.NewFsnotifyWatcher(path)
	w.OnChange(func() { v.Reload() })
	if err := w.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: file watching unavailable: %v\n", err)
	}
	defer w.Stop()

	v.Run()
}
