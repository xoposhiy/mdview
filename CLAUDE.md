# mdview

Go + WebView2 Markdown viewer for Windows. Source in `src/`, specs in `specs/`.

## Setup & build

```bash
cd src
go get github.com/jchv/go-webview2 github.com/yuin/goldmark
go mod tidy
go build -ldflags="-H windowsgui" -o ../mdview.exe .
cp internal/renderer/assets/mathjax.js ..    # displayed pages link this file
```

## Test

```bash
cd src
go test ./...
```

## Driving the app without a human (control interface)

mdview can be driven and inspected from outside, so a change can be checked in
the real window instead of only in unit tests. This matters most for the image
view: it is a native GDI control, so nothing in a browser can see it.

Start it with the interface on (a normal run has no listener):

```bash
go build -C src -ldflags="-H windowsgui" -o ../mdview.exe .
./mdview.exe -control-port=0 testdata/images.md &     # 0 = pick a free port
```

Then talk to it with `scripts/mdctl.sh` (a curl wrapper; the port comes from
`%TEMP%\mdview-control.port`):

```bash
scripts/mdctl.sh state              # file, mode (markdown/image), title, zoom, fit, pan, bg
scripts/mdctl.sh metrics            # per-stage timings: read, render, writeHTML, decode, pack, paint
scripts/mdctl.sh key right          # a key press; also: key r ctrl, key left alt
scripts/mdctl.sh wheel 120 400 300  # zoom in at a point
scripts/mdctl.sh drag 500 400 -200 -150
scripts/mdctl.sh open testdata/grid-1200x900.png
scripts/mdctl.sh resize 1200 900
scripts/mdctl.sh shot /c/tmp/window.png     # PNG of the window, WebView2 content included
scripts/mdctl.sh eval '(function(){return document.title})()'
scripts/mdctl.sh quit
```

- `key` goes to whichever view is on screen: the image control's own handler,
  or a `keydown` event in the page. The same names work for both.
- `eval` returns the value as JSON and waits for a promise, so it can measure
  something that takes time (frames painted, MathJax finishing).
- `shot` copies the pixels from the screen, so the window is raised first.
- Set `MDVIEW_CONFIG_DIR` to a scratch directory when driving the app, or the
  run will overwrite your own theme, font and background choices. The scripts
  below do this already.

Ready-made runs:

```bash
bash scripts/check-images.sh /c/tmp/mdvchk          # image view: zoom, pan, bg, navigation, Back
bash scripts/bench-startup.sh ./mdview.exe testdata/bench-big.md 3
```

`bench-startup.sh` reports time to first display, mdview's own stage timings,
the page's load time, frames per second while scrolling, and resize time.
Use it to check a change did not make display slower, instead of guessing.

`tests/links/verify.ps1` is the older way to do this: it injects keys with
`keybd_event` into whatever window has focus and waits fixed amounts of time.
Prefer the control interface for new checks — it can ask what is on screen
instead of sleeping and hoping.

## Key decisions

- Markdown is rendered in Go (goldmark) and pushed to WebView via `SetHtml` — no local HTTP server, no round-trip.
- Assets (CSS, HTML template) are embedded in the `renderer` binary via `//go:embed`; must live under `src/internal/renderer/assets/`.
- Hotkeys handled in JS (`keydown`), bound Go functions called via `webview.Bind` — `goExit`, `goReload`.
- `viewer.Reload` uses `w.Dispatch` to set HTML from a background goroutine back onto the UI thread.
- `watcher.FileWatcher` is an interface; v1 ships `NoOpWatcher`. Wire a real fsnotify watcher in v2 without touching viewer or renderer.
- `-H windowsgui` hides the console window when launched from Explorer; stderr still works from terminal.
- The initial file is read and rendered (or decoded) *before* the WebView2
  window is created, in `viewer.New`. That is what makes a bad path exit with
  a message instead of flashing an empty window, without rendering twice.
- The process declares itself Per-Monitor-V2 DPI aware, so every Win32 size is
  in physical pixels. Any size mdview passes to Win32 must therefore be scaled
  with `scaleForDPI` — see `defaultWindowWidth`.
- MathJax reaches a page in two steps, both of which exist to keep left/right
  navigation fast. A page gets MathJax only when it has a formula on it
  (`needsMathJax` looks for `\(`/`\[` in the finished HTML, so raw HTML counts
  too), and the pages the window shows link `mathjax.js` beside `mdview.exe`
  instead of carrying it. Inlining it in every page put 2 MB in each one, and
  WebView2 spent ~1.6 s per keypress compiling that again and tearing the old
  copy down — on documents with no formulas at all. Now: ~50 ms without
  formulas, ~100 ms with. `--export` and "Save as HTML" still inline it,
  because their output has to open on another machine; `viewer.Renderers` is
  what splits those two cases apart. A missing `mathjax.js` falls back to
  inlining, so a stray copy of the exe still shows formulas.
- The native image control (`imageControl`) is created the first time an image
  is displayed, not at startup, so a Markdown-only session pays nothing for
  it — in particular it does not get the subclassed parent `WndProc`.
