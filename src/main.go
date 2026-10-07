package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mdview/internal/config"
	"mdview/internal/control"
	"mdview/internal/renderer"
	"mdview/internal/viewer"
	"mdview/internal/watcher"
)

func main() {
	if os.Getenv("MDVIEW_DPI") != "0" {
		viewer.EnableDPIAwareness()
	}

	args, exportPath, controlPort := parseFlags(os.Args[1:])

	if exportPath != "" {
		exportHTML(args, exportPath)
		return
	}

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: mdview [-control-port=N] <file.md|.png|.jpg|.bmp|.gif>")
		os.Exit(1)
	}

	path := args[0]
	cfg := config.Load()

	// viewer.New reads and displays the file itself and reports what went
	// wrong, so there is nothing to check here first: a second read before
	// the window opens would only render or decode the file twice.
	v, err := viewer.New(path, cfg, viewer.Renderers{
		Display:    renderer.RenderLinked(mathJaxDir()),
		Standalone: renderer.RenderWithTheme,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if controlPort >= 0 {
		port, err := control.Serve(controlPort, v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot start the control server: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "control: listening on http://127.0.0.1:%d\n", port)
	}

	w := watcher.NewFsnotifyWatcher(path)
	w.OnChange(func() { v.Reload() })
	if err := w.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: file watching unavailable: %v\n", err)
	}
	defer w.Stop()

	v.Run()
}

// mathJaxDir returns the folder that displayed pages load MathJax from:
// %LOCALAPPDATA%\mdview. mdview writes the file there itself the first time a
// page needs it; see renderer.RenderLinked for why pages link the script
// instead of carrying it. The folder beside mdview.exe is not used, because
// it may be read-only, such as one under C:\Program Files.
func mathJaxDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "mdview")
}

// parseFlags splits argv into mdview's own flags and the positional arguments
// left over. controlPort is -1 when -control-port was not given, because 0 is
// itself a valid value: it asks the control server for a free port.
func parseFlags(argv []string) (args []string, exportPath string, controlPort int) {
	controlPort = -1
	for _, arg := range argv {
		switch {
		case strings.HasPrefix(arg, "--export="):
			exportPath = strings.TrimPrefix(arg, "--export=")
		case strings.HasPrefix(arg, "-control-port="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "-control-port="))
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: bad -control-port value: %v\n", err)
				os.Exit(1)
			}
			controlPort = n
		default:
			args = append(args, arg)
		}
	}
	return args, exportPath, controlPort
}

// exportHTML renders a Markdown file to a standalone HTML file and exits.
func exportHTML(args []string, outPath string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: mdview --export=<output.html> <file.md>")
		os.Exit(1)
	}
	mdPath := args[0]
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
	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
}
