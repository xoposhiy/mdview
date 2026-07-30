# External link handling and in-viewer navigation history

## Problem

Clicking a link in a rendered Markdown document navigates the WebView2 window
away from the document. The viewer becomes a browser showing an arbitrary web
page, with no way back to the file being read.

Links should instead open in the user's default browser. Links to other
Markdown files are the exception: those open inside mdview, which then needs a
back mechanism.

## Constraint that shapes the design

`Viewer.navigateHTML` writes the rendered page to a temp file and navigates to
`file:///%TEMP%/mdview_*.html`. Relative hrefs in the DOM therefore resolve
against `%TEMP%`, not against the Markdown file's directory: for a document at
`C:\docs\index.md` containing `[other](./other.md)`, the anchor's resolved
`href` property is `file:///C:/Users/.../Temp/other.md` — the wrong file.

So JavaScript must send the **raw** `getAttribute('href')` and Go must resolve
it against `filepath.Dir(v.filePath)`. This mirrors `resolveImageSrcs` in the
renderer, which exists for the same reason.

A second constraint: `go-webview2`'s exported `WebView` interface offers only
`Bind`, `Eval`, `Init`, `Navigate`, `SetHtml`, `Dispatch` and friends. The
`AddNavigationStarting` / `AddNewWindowRequested` COM procs exist in
`pkg/edge/corewebview2.go` but no exported Go registration API wraps them.
Interception therefore happens in JavaScript and calls into bound Go functions
— the pattern the existing hotkeys already use.

## Behaviour

| Link | Result |
|---|---|
| `#heading` | Scrolls within the page (unchanged default behaviour) |
| `https://…`, `http://…`, `mailto:…`, `obsidian://…`, any scheme | Default browser / registered handler, via `ShellExecuteW` |
| `./other.md`, `sub/Other.MD`, `C:/abs/x.md` | Opens **inside mdview** |
| `spec.pdf`, `image.png`, any other local file | Registered application, via `ShellExecuteW` |

Navigation history is unified: every file change is recorded, whether caused by
a followed link or by the existing `←`/`→` prev/next-file keys. Back restores
both the file and the scroll position it was left at.

```
open  A.md
→            history: [A]         now B
→            history: [A, B]      now C
click link   history: [A, B, C]   now X
Backspace    history: [A, B]      now C  (scroll restored)
Backspace    history: [A]         now B
Backspace    history: []          now A
Backspace    (no-op)
```

Back is triggered by `Backspace`, `Alt+←`, or a `←` button at the top-left of
the window that is hidden while the history is empty.

## Components

### `src/internal/viewer/links.go` (new)

Pure classification, no WebView dependency:

```go
type linkKind int // linkInPage, linkMarkdown, linkExternal

func classifyLink(href, baseDir string) (linkKind, string)
```

Rules, in order:

1. Empty href, or href starting with `#` → `linkInPage`, target = href unchanged.
2. Matches `^[A-Za-z][A-Za-z0-9+.-]+:` → `linkExternal`, target = href verbatim.
   The scheme requires **two or more** characters so that a Windows absolute
   path such as `C:/docs/x.md` is not mistaken for a `C:` scheme.
3. Otherwise a local path. Strip any `#fragment` and `?query`, percent-decode,
   and join onto `baseDir` when the path is relative. Extension `.md` or
   `.markdown` (case-insensitive) → `linkMarkdown`; anything else →
   `linkExternal`, both with the resolved absolute path as target.

### `src/internal/viewer/history.go` (new)

```go
type histEntry struct {
    path    string
    scrollY int
}

type history []histEntry

func (h *history) push(path string, scrollY int)
func (h *history) pop() (histEntry, bool) // false when empty
func (h history) canGoBack() bool
```

### `src/internal/viewer/viewer.go` (modified)

`navigate(offset)` currently inlines its own file-loading tail: read file, set
`filePath`, reset `scrollY`, render, dispatch title and HTML. That tail is
extracted so link-follow and back can reuse it:

```go
// show renders path into the window at the given scroll position.
// Sets v.filePath and v.scrollY. Does not touch history.
func (v *Viewer) show(path string, scrollY int) error

// open records the current document in history, then shows path at the top.
// Returns a message for the user, or "" on success.
func (v *Viewer) open(path string) string

// back pops one history entry and shows it at its recorded scroll position.
// Returns a message for the user, or "" on success / empty history.
func (v *Viewer) back() string
```

`open` and `back` return a user-facing message rather than an `error` because
both are consumed directly by string-returning bindings; only `show` deals in
`error`.

`open` captures the current `filePath`/`scrollY` first, calls `show`, and
pushes onto history **only if `show` succeeded** — so a broken link leaves both
the displayed document and the history untouched:

```go
func (v *Viewer) open(path string) string {
    prevPath, prevScroll := v.filePath, v.scrollY
    if err := v.show(path, 0); err != nil {
        return "Not found: " + filepath.Base(path)
    }
    v.history.push(prevPath, prevScroll)
    return ""
}
```

`navigate(±1)` and link-follow both call `open`, which is what makes the
history unified. `Reload` (F5) calls `show(v.filePath, v.scrollY)` and pushes
nothing.

`navigate` returns `open`'s message, and the `goNext`/`goPrev` bindings change
from `func()` to `func() string` so the page toasts them the same way it toasts
link failures. Without this, `←`/`→` would be the only navigation path that
fails silently.

`Viewer` gains one field: `history history`.

The scroll position pushed onto history is `v.scrollY`, maintained by the
existing `goSaveScroll` binding with its 100 ms debounce. A position may
therefore be up to 100 ms stale; this is acceptable.

Scroll restoration needs no new mechanism: `show` passes `scrollY` to
`renderFn`, which the template already emits as `window.scrollTo(0, {{.ScrollY}})`.

### Bindings (new)

Each returns a `string` so the page can show a toast on failure, following the
existing `goSaveHTML` pattern.

- **`goOpenLink(href string) string`** — classifies `href` against
  `filepath.Dir(v.filePath)`, then either calls `v.open(target)` or
  `ShellExecuteW("open", target, …)` using the `procShellExecute` proc that
  `openInEditor` already declares. A `ShellExecuteW` return value `<= 32` is an
  error: log via `applog.Write` and return `"Cannot open: …"`. A `.md` target
  that cannot be read returns `"Not found: other.md"`. Success returns `""`.

  One binding rather than two: the page should not need to know the taxonomy,
  and classification stays in a single testable place.

- **`goBack() string`** — delegates to `v.back()`. Returns `""` on empty
  history so that Backspace is simply inert there. `Esc` keeps sole ownership
  of quitting.

- **`goCanGoBack() bool`** — queried once per page load to decide the back
  button's visibility.

`goCanGoBack` is why the renderer is untouched by this change: no `CanGoBack`
field on `pageData`, and no fifth parameter added to the already four-parameter
`renderFn` signature. History depth stays entirely inside the viewer.

### `src/internal/renderer/assets/template.html` (modified)

Per the project's CSS rule, all styling stays inline in this template.

- Stub declarations for `goOpenLink`, `goBack`, `goCanGoBack` alongside the
  existing `typeof … === 'undefined'` block, so the page still works when
  opened outside the app. These stubs return `Promise.resolve('')` /
  `Promise.resolve(false)`, matching the existing `goSaveHTML` stub rather than
  the bare `function() {}` stubs used for void bindings.

- Because `goNext`/`goPrev` now return a value, their stubs change to
  `Promise.resolve('')` too, and their two call sites in the keydown handler
  become `goPrev().then(function(msg) { if (msg) showToast(msg); })`.

- A delegated click handler on `document` — not on `.content`, so links inside
  the frontmatter table are covered too:

  ```js
  document.addEventListener('click', function(e) {
      var a = e.target.closest && e.target.closest('a[href]');
      if (!a) return;
      var href = a.getAttribute('href');
      if (!href || href.charAt(0) === '#') return; // in-page anchor: let it scroll
      e.preventDefault();
      goOpenLink(href).then(function(msg) { if (msg) showToast(msg); });
  });
  ```

  `preventDefault` also neutralises `target="_blank"`, which is reachable
  because the renderer enables `ghtml.WithUnsafe()`.

- A `#back-btn` mirroring `#settings-btn` but at `left: 16px`, `display: none`
  until the `load` handler's `goCanGoBack()` call reports otherwise. Every
  navigation is a full page load, so load time is the only moment this needs
  evaluating.

- Key handling, with placement that is easy to get wrong:
  - **`Alt+←` must be handled above** the existing
    `if (e.ctrlKey || e.altKey || e.metaKey) return;` line, alongside F5 and
    Ctrl+S — otherwise that early return swallows it.
  - **Plain `Backspace` goes below** that line and below the
    `INPUT`/`SELECT`/`TEXTAREA` guard, so the font-size slider keeps its own
    Backspace handling. Calls `preventDefault`.
  - Plain `←`/`→` continue to mean prev/next file; no conflict, since the
    modifier check runs first.

- A help-overlay row: `Backspace, Alt+←` → `Back`.

## Error handling

| Situation | Result |
|---|---|
| `ShellExecuteW` returns `<= 32` | `applog.Write`, toast `"Cannot open: …"` |
| Linked `.md` file unreadable | Toast `"Not found: other.md"`, current document stays |
| Back onto a file deleted meanwhile | Toast `"Not found: …"`; the entry is consumed, so pressing back again goes further back |
| Back with empty history | Nothing happens |

## Testing

Table-driven tests in the style of the existing `TestTitleFromPath`, both fully
headless:

- **`classifyLink`** — `#sec`, empty string, `https://x`, `mailto:a@b`,
  `obsidian://open?x`, `./other.md`, `sub/Other.MD`, `C:/abs/x.md`, `spec.pdf`,
  `other%20file.md`, `other.md#sec`.
- **`history`** — push/pop ordering, `pop` on empty returning `false`,
  `canGoBack` transitions.

`show`, `open` and `back` need a live WebView and stay thin glue over the
tested parts.

## Out of scope

- `other.md#section` loads the file at the top; the fragment is dropped rather
  than scrolled to.
- No forward navigation. `Alt+→` stays unbound.
- Middle-click and Ctrl+click are not intercepted; only `click` is.
