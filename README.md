# mdview

A fast, single-binary Markdown viewer for Windows. Written in Go, renders with
[goldmark](https://github.com/yuin/goldmark), displays through WebView2 — no
local HTTP server, no browser tab, no Electron.

```
mdview file.md
```

## Features

**Markdown**

- Full GitHub Flavored Markdown: tables, task lists, strikethrough, autolinks
- LaTeX math via MathJax — inline `$...$` and display `$$...$$`, rendered
  without mangling underscores or braces
- Obsidian-style YAML frontmatter shown as a properties table above the
  document; nested keys flatten to `source.file`, list values render as pills
- Local images resolved relative to the file
- Sortable table headers — click to sort, click again to reverse
- Over-wide tables get their own horizontal scroller instead of breaking the
  reading column

**Reading**

- 7 themes: Cream, GitHub, Solarized Light, Dark, Solarized Dark, Dracula, Nord
- 4 font choices (sans, serif, reading, mono) and adjustable body size, 14–30 px
- Narrow reading column by default, toggleable to full width
- Preferences persist to `%APPDATA%\mdview\config.json`

**Workflow**

- Live reload — the file is watched, and edits re-render with your scroll
  position preserved
- Arrow keys walk the other `.md` files in the same directory
- Open the source in your editor (`%EDITOR%` / `%VISUAL%`, else the shell's
  edit verb, else Notepad)
- Save the rendered page as a standalone HTML file
- `--export=out.html file.md` renders headlessly and exits, no window

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `←` / `→` | Previous / next file in the directory |
| `S` | Appearance panel (theme, font, size) |
| `D` | Toggle light / dark theme |
| `W` | Toggle wide / narrow layout |
| `E` | Open in editor |
| `Ctrl+S` | Save as HTML |
| `F5`, `Ctrl+R` | Reload file |
| `?`, `H` | Show / hide shortcuts |
| `Esc` | Close panel, or quit |

## Build

Requires Go 1.22+ and the WebView2 runtime (preinstalled on Windows 11).

```bash
cd src
go build -ldflags="-H windowsgui" -o ../mdview.exe .
```

`-H windowsgui` suppresses the console window when launching from Explorer.

## Test

```bash
cd src
go test ./...
```

Browser-level table tests live in `tests/` and run under Playwright:

```bash
cd tests
npm install
npm test
```
