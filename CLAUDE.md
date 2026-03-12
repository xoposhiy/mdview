# mdview

Go + WebView2 Markdown viewer for Windows. Source in `src/`, specs in `specs/`.

## Setup & build

```bash
cd src
go get github.com/jchv/go-webview2 github.com/yuin/goldmark
go mod tidy
go build -ldflags="-H windowsgui" -o ../mdview.exe .
```

## Test

```bash
cd src
go test ./...
```

## Key decisions

- Markdown is rendered in Go (goldmark) and pushed to WebView via `SetHtml` — no local HTTP server, no round-trip.
- Assets (CSS, HTML template) are embedded in the `renderer` binary via `//go:embed`; must live under `src/internal/renderer/assets/`.
- Hotkeys handled in JS (`keydown`), bound Go functions called via `webview.Bind` — `goExit`, `goReload`.
- `viewer.Reload` uses `w.Dispatch` to set HTML from a background goroutine back onto the UI thread.
- `watcher.FileWatcher` is an interface; v1 ships `NoOpWatcher`. Wire a real fsnotify watcher in v2 without touching viewer or renderer.
- `-H windowsgui` hides the console window when launched from Explorer; stderr still works from terminal.
