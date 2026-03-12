package viewer

import (
	"os"
	"path/filepath"

	webview "github.com/jchv/go-webview2"
)

// TitleFromPath returns the window title for the given file path.
func TitleFromPath(path string) string {
	return filepath.Base(path) + " — mdview"
}

// Viewer manages the WebView2 window.
type Viewer struct {
	w        webview.WebView
	filePath string
	renderFn func([]byte) (string, error)
}

// New creates a Viewer, binds hotkey handlers, and sets the initial HTML.
// renderFn is called on reload to convert file content to HTML.
func New(filePath, initialHTML string, renderFn func([]byte) (string, error)) *Viewer {
	w := webview.NewWithOptions(webview.WebViewOptions{AutoFocus: true})
	w.SetTitle(TitleFromPath(filePath))
	w.SetSize(960, 720, webview.HintNone)

	v := &Viewer{w: w, filePath: filePath, renderFn: renderFn}

	w.Bind("goExit", func() {
		w.Terminate()
	})

	w.Bind("goReload", func() {
		v.reload()
	})

	w.SetHtml(initialHTML)
	return v
}

// Run starts the event loop. Blocks until the window is closed.
func (v *Viewer) Run() {
	v.w.Run()
	v.w.Destroy()
}

func (v *Viewer) reload() {
	content, err := os.ReadFile(v.filePath)
	if err != nil {
		return
	}
	html, err := v.renderFn(content)
	if err != nil {
		return
	}
	v.w.Dispatch(func() {
		v.w.SetHtml(html)
	})
}
