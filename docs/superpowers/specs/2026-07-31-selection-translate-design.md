# Translate the selection

## Problem

Reading notes in a foreign language in mdview means leaving mdview: copy the
paragraph, switch to a browser, paste it into a translator, switch back. The
viewer already holds the rendered text and already has a JS↔Go channel.
Selecting a passage and pressing one button or one key must be enough.

## Constraints that shape the design

**The page has a `file://` origin.** `navigateHTML` (`src/internal/viewer/viewer.go`)
writes the rendered HTML to a temp file and navigates to it, so the document
has an opaque (`null`) origin. A `fetch` from the page to DeepL would work only
if DeepL returned `Access-Control-Allow-Origin: *`. Also, the page would need
the API key, and the page lives in `%TEMP%` for the lifetime of the process.
**The HTTP request belongs in Go.** Go's default transport also gives
`HTTP_PROXY` support for free.

**Bound functions run on the UI thread.** `msgcb`
(`src/vendor/github.com/jchv/go-webview2/webview.go`) calls the binding
synchronously and only sends the promise result back through `Dispatch`. A
blocking HTTP call in a binding would freeze the window for the whole request,
and for the full timeout when the network is down. The translation binding must
return at once and deliver its result later.

**Esc quits the application** when no overlay is open (the `keydown` handler in
`template.html`). The translation card must take part in Esc handling, or the
natural "close this popup" key closes mdview.

**`config.Save` writes the whole struct from memory.** The user edits the
translation settings by hand while mdview may be running. If mdview then saves
a theme change from its old in-memory copy, it erases the key the user just
added. See [Configuration](#configuration).

## Behaviour

1. The user selects text in the document, with the mouse or with the keyboard.
2. About 150 ms after the selection stops changing, a small `文A` button
   appears under the end of the selection. Its tooltip is `Translate (T)`.
3. The user clicks the button or presses `T`.
4. A spinner replaces the icon inside the button.
5. When the reply arrives, the button turns into a card at the same place. The
   card shows the translation.

| Situation | Result |
|---|---|
| Text selected, click `文A` or press `T` | Translation in a card under the selection |
| Nothing selected, press `T` | Nothing happens |
| Detected source language equals `to` | A second request translates into `fallback` |
| No API key configured | The card shows a hint with the full path to `config.json` |
| Selection over `MaxChars` | The card shows the actual and the allowed length; no request is sent |
| Same text selected again | Result comes from the in-process cache; no request, no quota spent |
| New selection in the document, click outside the card, Esc, page change | The card or the button closes |
| Selection or click inside the card | The card stays open |
| Scroll | The card and the button move with the text |

### Languages

The user sets two languages: `to` (default `RU`) and `fallback` (default `EN`).
DeepL detects the source language; mdview never sends `source_lang`.

DeepL returns `detected_source_language`. When that language equals `to`, mdview
sends the same text again with `target_lang` set to `fallback`. Example: with
`to=RU` and `fallback=EN`, English text becomes Russian and Russian text becomes
English. The second request costs quota a second time. This happens only for
text that is already in the target language, so the cost is acceptable.

Compare the language codes by their primary part: DeepL accepts `EN-GB` as a
target but reports `EN` as the detected source. `to=EN-GB` and detected `EN`
count as equal.

## UI

### The button

- Shown when the selection is non-empty and lies inside `.content`, after the
  selection has not changed for about 150 ms. Not shown while the mouse is still
  dragging, so the button does not flicker.
- A small pill with the icon `文A` and no text. Opacity about 0.7, 1 on hover.
- Position:
  - Vertical: 6 px under the last line of the selection (the last rect from
    `getRangeAt(0).getClientRects()`).
  - Horizontal: at the pointer position of the `mouseup` that ended the
    selection; for a keyboard selection, at the right edge of the last rect.
  - If there is no room under the last line, show the button above the first
    line.
  - Position the button in document coordinates (`position: absolute` relative
    to the document), so it scrolls with the text.
- The button listens on **`mousedown` with `preventDefault()`**. A `click`
  listener fires after the selection has already collapsed, and there is
  nothing left to translate.

### Loading

A spinner replaces the icon inside the button. Do not show an empty card first:
DeepL usually replies in less than half a second, and an empty card would only
flash.

### The card

- The top-left corner of the card stays where the top-left corner of the button
  was. Shift the card horizontally so it stays inside the viewport.
- Width: the width of the selection, clamped to 280–520 px.
- Height: at most 40% of the viewport, then `overflow: auto`.
- If there is less room below than above, open the card upward, with its bottom
  edge above the first line of the selection.
- Header row, in small muted text: the direction (`EN → RU`), a copy button,
  and a close button `×`.
- Body: the translation, in the document font, 1 px smaller than the body text.
  Insert the text with `textContent`, never `innerHTML`.
- Colors come from the theme variables `--box-bg`, `--box-border`, `--fg` and
  `--link`, which every theme block already defines. No theme block changes.
- Hidden in print (add it to the existing `@media print` rule).
- All CSS goes inline in the existing `<style>` block of `template.html`
  (`src/internal/renderer/AGENTS.md`).

### Closing

- A new non-empty selection in the document closes the card and shows a new
  button for the new selection.
- A selection change that lies inside the card does not close the card. The
  user can select part of the translation and copy it.
- `mousedown` outside the card and outside the button closes the card.
- Esc closes the card or the button. The Esc branch in `keydown` checks the
  card before `anyOverlayOpen()`, so Esc never quits while the card is open.
- Going to another page closes the card (the page is replaced anyway; the Go
  side also drops late replies, see `transSeq`).

### Errors

Errors appear inside the card, in a muted red, in the same place the
translation would appear. The user looks at that place, so the message goes
there, not into a toast. For network errors and rate limits the card also shows
a `Retry` button.

### Keyboard

`T` joins the `keydown` handler. It sits below the existing guard for `INPUT`,
`SELECT` and `TEXTAREA`. With no selection, `T` does nothing.

The help overlay gains a row: `T` — `Translate selection`.

## DeepL API

Verified against `developers.deepl.com` on 2026-07-31.

- `POST https://api-free.deepl.com/v2/translate` for Free keys,
  `https://api.deepl.com/v2/translate` for Pro.
- Header `Authorization: DeepL-Auth-Key <key>`.
- JSON body. `text` is an **array** of strings:
  `{"text": ["…"], "target_lang": "RU"}`. No `source_lang`.
- Response: `{"translations":[{"detected_source_language":"EN","text":"…"}]}`.
  Read only `translations[0]`.
- Request size limit 128 KiB.

Free keys end with `:fx`, so mdview picks the host from the key. A user who
upgrades to Pro pastes the new key, and the client follows it.

## Components

### `src/internal/translate/` (new package)

Standard library only; no new dependencies.

```go
type Result struct {
    Text   string // the translation
    Source string // detected source language, e.g. "EN"
    Target string // the language actually used, e.g. "RU"
}

type Translator interface {
    Translate(ctx context.Context, text, target string) (Result, error)
}
```

`translate.go` holds the parts that must be unit-testable, because the viewer's
own code is not:

```go
const MaxChars = 5000

var (
    ErrNoKey     = errors.New("no api key")
    ErrEmpty     = errors.New("empty text")
    ErrAuth      = errors.New("auth failed")
    ErrQuota     = errors.New("quota exceeded")
    ErrRateLimit = errors.New("rate limited")
    ErrTooLong   = errors.New("text too long")
    ErrTransient = errors.New("service unavailable")
)

type TooLongError struct{ Chars int } // Is(ErrTooLong) == true

func Precheck(text string) error                  // empty, or over MaxChars
func Message(err error, configPath string) string // English text for the card; "" for nil
func Retryable(err error) bool                    // true for ErrRateLimit, ErrTransient, network errors
func SameLanguage(a, b string) bool               // compares primary codes: "EN-GB" == "EN"

// WithFallback translates into to; if the detected source equals to, it
// translates again into fallback.
func WithFallback(ctx context.Context, t Translator, text, to, fallback string) (Result, error)
```

- `MaxChars` counts **runes** (`utf8.RuneCountInString`), so the number agrees
  with what the user calls a character. It is a readability cap, not an API
  limit. Its real job: make an accidental Ctrl+A cost nothing.
- `Precheck` rejects empty and whitespace-only text. The page never sends such
  text; this is a guard on the package boundary.
- `Message(ErrNoKey, path)` names the file and the field:
  `No DeepL key. Add "translate.deeplKey" to <path>`.
- `WithFallback` skips the second request when `fallback` is empty or equals
  `to`.

`deepl.go` holds `DeepL`, built with `NewDeepL(key string)`:

- Picks the host from the `:fx` suffix. Store the resolved endpoint on the
  struct, so a test can check it without a request.
- Uses `http.Client{Timeout: 10 * time.Second}`. The zero value has no timeout,
  and the spinner would then spin forever.
- Maps status codes:

| Status | Error |
|---|---|
| 200 | — |
| 403 | `ErrAuth` |
| 413 | `ErrTooLong` |
| 429, 529 | `ErrRateLimit` |
| 456 | `ErrQuota` |
| 500, 504 | `ErrTransient` |
| other | wrapped, with DeepL's `message` field when the body has one |

`cache.go` wraps any `Translator`:

```go
func Cached(t Translator) Translator
```

A `map` keyed by `(text, target)` behind a `sync.Mutex`. The mutex is required,
because lookups happen on the goroutine, not on the UI thread. Errors are not
cached. No eviction: entries live for the process. Because the key includes the
target, both requests of a fallback pair are cached.

### Configuration

`src/internal/config/config.go` gains a nested object:

```go
type Translate struct {
    DeepLKey string `json:"deeplKey"`
    To       string `json:"to"`       // default "RU"
    Fallback string `json:"fallback"` // default "EN"
}

type Config struct {
    // ... existing fields ...
    Translate Translate `json:"translate"`
}
```

In `config.json`:

```json
"translate": { "deeplKey": "…:fx", "to": "RU", "fallback": "EN" }
```

The file stays where it is: `%APPDATA%\mdview\config.json`, or
`$MDVIEW_CONFIG_DIR\config.json`. There is no UI for these settings; the user
edits the file.

Two changes make hand edits safe:

1. **Read on every translation.** `goTranslate` calls `config.Load()` and uses
   its `Translate` field. One small `ReadFile` per click costs nothing, and an
   edit takes effect without a restart.
2. **`Save` updates only its own fields.** `Save` reads the file from disk into
   a `map[string]json.RawMessage`, replaces the keys of the appearance fields
   (`theme`, `font`, `fontSize`, `imageBg`), and writes the map back. Keys that
   `Save` does not own, `translate` among them, stay as they are on disk. This
   also protects any other field the user adds by hand.

`config.Path()` becomes exported, so the "no key" message can name the file.

### `src/internal/viewer/viewer.go` (modified)

`Viewer` gains `translator translate.Translator`, `translatorKey string` and
`transSeq int`. The viewer builds the translator lazily, and rebuilds it when
the key in the file differs from `translatorKey`. The cache therefore lives as
long as the key stays the same.

One binding:

```go
w.Bind("goTranslate", func(text string) string {
    cfg := config.Load().Translate
    if cfg.DeepLKey == "" {
        return errPayload(translate.ErrNoKey)
    }
    if err := translate.Precheck(text); err != nil {
        return errPayload(err)
    }
    tr := v.translatorFor(cfg.DeepLKey)
    v.transSeq++
    seq := v.transSeq
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
        defer cancel()
        res, err := translate.WithFallback(ctx, tr, text, cfg.To, cfg.Fallback)
        payload := marshalResult(res, err)
        v.w.Dispatch(func() {
            if seq != v.transSeq {
                return // superseded, or the page has changed
            }
            v.w.Eval("mdviewTranslateDone(" + payload + ")")
        })
    }()
    return "" // "" means "request started"
})
```

The payload is JSON:
`{"ok": bool, "text": "…", "source": "EN", "target": "RU", "msg": "…", "retry": bool}`.
The fast checks return the same payload synchronously through the binding's
promise; the page shows it in the card, the same way as a late result.

- The overall 25 s context covers two DeepL requests of 10 s each.
- **`transSeq` needs no mutex.** `goTranslate` writes it, and the `Dispatch`
  closure reads it; both run on the UI thread. The goroutine reads only its own
  copy. The same holds for `translator` and `translatorKey`, which is why the
  goroutine captures `tr` before it starts.
- `show` also increments `transSeq`, inside its existing `Dispatch` closure
  next to `navigateHTML`. This drops a reply that arrives after the user went
  to another page.
- Putting JSON into an `Eval` string is safe: `encoding/json` escapes U+2028
  and U+2029, the two characters that are legal in JSON strings but not in
  JavaScript string literals.

### `src/internal/renderer/assets/template.html` (modified)

- Markup: `#translate-btn` and `#translate-card`, hidden by default.
- A `selectionchange` listener, debounced about 150 ms, places or hides the
  button (see [The button](#the-button)). Ignore selection changes inside the
  card.
- `mdviewTranslateDone(res)` fills the card only while the button is in its
  loading state. This covers a case the Go sequence number cannot see: the user
  closed the card in JS, and Go never heard about it.
- The copy button calls `navigator.clipboard.writeText`. If that fails on the
  `file://` origin, fall back to a hidden `<textarea>` and
  `document.execCommand('copy')`.
- The `Retry` button calls `goTranslate` again with the same text.

**The key never reaches the page.** No binding returns it, and no markup holds
it. The page is written to `%TEMP%` and into every file saved with Ctrl+S.

## Error handling

| Situation | Message in the card | Retry |
|---|---|---|
| No key configured | `No DeepL key. Add "translate.deeplKey" to <path>` | no |
| Over `MaxChars` | `Selection is too long: N characters, the limit is 5000` | no |
| 403 | `DeepL rejected the key` | no |
| 456 | `DeepL monthly quota is used up` | no |
| 429, 529 | `Too many requests. Try again later` | yes |
| 413 | `Selection is too long for DeepL` | no |
| 500, 504 | `DeepL is temporarily unavailable` | yes |
| Timeout, DNS, offline | `Cannot reach DeepL` | yes |
| Other status | `DeepL error: <message from DeepL>` | no |

Every case reads differently, because "error" alone does not tell the user
whether to fix the key, wait, or select less.

## Testing

`src/internal/translate/deepl_test.go`, against `httptest.Server`:

- Request shape: method, path, `Authorization` header, a body whose `text` is
  an array and whose `target_lang` is the requested target, no `source_lang`.
- Response parsing: `text` and `detected_source_language`; with more than one
  translation, only the first one is read.
- Each status in the table maps to its error, checked with `errors.Is`.
- A 400 body with `message` puts that text into the wrapped error.
- Host selection: a key ending with `:fx` targets `api-free`, other keys target
  `api`.

`src/internal/translate/translate_test.go`:

- `Precheck` on empty, whitespace-only, exactly `MaxChars`, and `MaxChars+1`.
- `Precheck` counts runes: 3000 Cyrillic characters pass, although they take
  6000 bytes.
- `Message` returns a non-empty string for every exported error and for an
  unknown error, `""` for `nil`, and names the path for `ErrNoKey`.
- `Retryable` for each error.
- `SameLanguage`: `EN`/`EN-GB` equal, `en`/`EN` equal, `RU`/`EN` not equal.
- `WithFallback`, with a fake `Translator`: one call when the source differs
  from `to`; a second call into `fallback` when it equals `to`; no second call
  when `fallback` is empty or equals `to`; an error from the second call is
  returned.

`src/internal/translate/cache_test.go`:

- A second call with the same text and target does not reach the wrapped
  translator; a different text or a different target does.
- An error is not cached; the next call retries.
- Concurrent calls from several goroutines, so `-race` covers the mutex.

`src/internal/config/config_test.go`:

- `Load` of a file without `translate` gives the defaults `RU` and `EN`.
- `Save` keeps a `translate` object that exists on disk but not in memory.
- `Save` keeps an unknown top-level key.
- `Save` into a missing file creates it.

Check in the real window through the control interface
(`scripts/mdctl.sh`, with `MDVIEW_CONFIG_DIR` set to a scratch directory):

- Select text with `eval`, then check that `#translate-btn` is visible and
  placed under the selection.
- `key t` with no key configured: the card shows the hint with the path.
- `key escape` with the card open: the card closes, and `state` still answers
  (the application did not quit).
- `shot` of the card near the bottom and right edges, in a light and a dark
  theme.
- A real DeepL request only when a key is present in the environment. Do not
  commit a key.

## Privacy

The selected text goes to DeepL, and only after the user clicks the button or
presses `T`. On the Free tier, DeepL's terms permit using submitted text to
improve their models; the Pro tier excludes this. With no key configured,
nothing leaves the machine.

## Out of scope

- A settings UI for the key and the languages.
- Any provider other than DeepL. The `Translator` interface is the one place a
  second provider would attach. Do not add a `provider` setting until it exists.
- Sending HTML (`tag_handling=html`) to keep formatting, code and links. v1
  sends `selection.toString()`.
- A usage or quota counter (`/v2/usage`).
- A portable mode with `config.json` next to `mdview.exe`.
- Translating a whole document, or replacing the original text in place.
- A history of translations, or keeping the cache across runs.
- Automatic retry with backoff. The `Retry` button covers it.
