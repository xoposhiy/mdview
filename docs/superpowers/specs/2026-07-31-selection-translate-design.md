# Translate the selection into Russian

## Problem

Reading English technical notes in mdview means leaving mdview: copy the
paragraph, switch to a browser, paste it into a translator, switch back. The
viewer already holds the rendered text and already has a JS↔Go channel. Selecting
a passage and pressing one key should be enough.

## Constraints that shape the design

**The page has a `file://` origin.** `navigateHTML` (`src/internal/viewer/viewer.go:118`)
writes the rendered HTML to a temp file and navigates to it, which gives the
document an opaque (`null`) origin. A `fetch` from the page to DeepL would only
be readable if DeepL returned `Access-Control-Allow-Origin: *`, which is not
something to build on. More decisively, the API key would have to be present in
the page to make that call, and the page lives in `%TEMP%` for the lifetime of
the process. **The HTTP request belongs in Go**, which also gets `HTTP_PROXY`
support for free from `net/http`'s default transport.

**Bound functions run on the UI thread.** `msgcb`
(`src/vendor/github.com/jchv/go-webview2/webview.go:139`) calls `callbinding`
synchronously and only marshals the promise resolution back through `Dispatch`.
Every existing binding is therefore cheap — the heaviest, `goSaveHTML`, is a
file write. A blocking HTTP call in a binding would freeze the window for the
duration of the request, and for the full timeout when the network is down. The
translation binding must return immediately and deliver its result out of band.

**Esc quits the application** when no overlay is open (`template.html:506`). Any
new panel that does not participate in `anyOverlayOpen()` / `closeOverlays()`
turns the natural "dismiss this popup" gesture into "close mdview".

## Behaviour

Select text, then either press `T` or click the small `Я` button that appears
below the selection. A popover opens under the selection showing `…`, then the
Russian translation.

| Situation | Result |
|---|---|
| English (or any) text selected | Russian translation in a popover under the selection |
| Nothing selected | Nothing happens |
| No API key configured | Toast: `Ключ DeepL не настроен — задайте его в настройках (S)` |
| Selection over `maxChars` | Toast naming the actual and permitted length; no request sent |
| Same text selected again | Served from the in-process cache; no request, no quota spent |
| Esc, click outside, or a new selection | Popover closes |

Target language is always Russian, with no check on the input. Text that is
already Russian is sent to DeepL and comes back essentially unchanged. This is a
deliberate simplification: the key always means one thing, and the alternative —
guessing direction from the share of Cyrillic — makes the key's effect depend on
content in a way that is hard to predict mid-document.

## DeepL API

Verified against `developers.deepl.com` on 2026-07-31.

- `POST https://api-free.deepl.com/v2/translate` for Free keys,
  `https://api.deepl.com/v2/translate` for Pro.
- Header `Authorization: DeepL-Auth-Key <key>`.
- JSON body. `text` is an **array** of strings, not a string:
  `{"text": ["…"], "target_lang": "RU"}`. Source language is omitted so DeepL
  detects it.
- Response: `{"translations":[{"detected_source_language":"EN","text":"…"}]}`.
  Only `translations[0].text` is read.
- Request size limit 128 KiB.

Free keys carry the suffix `:fx` (documented on the authentication page), so the
host is chosen from the key rather than from a separate setting. A user who
upgrades to Pro pastes the new key and the client follows it, instead of failing
with a confusing 403 against the wrong host.

## Components

### `src/internal/translate/` (new package)

The `watcher.FileWatcher` pattern: interface first, so the viewer never names a
concrete client and tests never touch the network.

```go
type Translator interface {
    Translate(ctx context.Context, text string) (string, error)
}
```

`translate.go` also holds the pieces that must be unit-testable, because the
viewer's own code cannot be:

```go
const MaxChars = 5000

var (
    ErrNoKey     = errors.New("no api key")
    ErrAuth      = errors.New("auth failed")
    ErrQuota     = errors.New("quota exceeded")
    ErrRateLimit = errors.New("rate limited")
    ErrTooLong   = errors.New("text too long")
    ErrTransient = errors.New("service unavailable")
)

func Precheck(text string) error   // empty or over MaxChars
func Message(err error) string     // Russian text for the popover; "" for nil
```

`Precheck` rejects empty and whitespace-only text, though the page should never
send it: the `T` handler and the button both check the selection first. It is a
guard on the package boundary, not the mechanism behind "nothing selected does
nothing".

`MaxChars` is 5000 **runes**, counted with `utf8.RuneCountInString` so the
message agrees with what the user would call a character. It is a readability
cap, not an API one — 128 KiB is far larger — and its real job is to make an
accidental Ctrl+A cost nothing.

`deepl.go` holds `DeepL`, built with `NewDeepL(key string)`. It picks the host
from the `:fx` suffix, sets `http.Client{Timeout: 10 * time.Second}` — the zero
value has no timeout at all, which is exactly the failure that would leave the
popover spinning forever — and maps status codes:

| Status | Error |
|---|---|
| 200 | — |
| 403 | `ErrAuth` |
| 413 | `ErrTooLong` |
| 429, 529 | `ErrRateLimit` |
| 456 | `ErrQuota` |
| 500, 504 | `ErrTransient` |
| other | wrapped, with DeepL's `message` field when the body carries one |

`cache.go` wraps any `Translator`:

```go
func Cached(t Translator) Translator
```

A `map[string]string` behind a `sync.Mutex` — the mutex is required because
lookups happen on the goroutine, not the UI thread. No eviction: entries live
for the process, and the upper bound is how many distinct passages one person
selects in one sitting.

### `src/internal/config/config.go` (modified)

One field:

```go
DeepLKey string `json:"deeplKey"`
```

`Load` already unmarshals over defaults (`config.go:38-40`), so existing
`config.json` files without the field keep working untouched.

### `src/internal/viewer/viewer.go` (modified)

`Viewer` gains two fields: `translator translate.Translator`, built in `New`
from `prefs.DeepLKey` and `nil` when the key is empty, and `transSeq int`.

`New`'s signature does not change. The viewer already receives `config.Config`
and already owns preference writes (`goSaveTheme` and friends, `viewer.go:206-219`),
so the key follows the same path and `main.go` is untouched.

Three bindings:

```go
w.Bind("goHasTranslateKey", func() bool { return v.translator != nil })

w.Bind("goSaveTranslateKey", func(key string) string {
    key = strings.TrimSpace(key)
    v.prefs.DeepLKey = key
    config.Save(v.prefs)
    if key == "" { v.translator = nil; return "Ключ удалён" }
    v.translator = translate.Cached(translate.NewDeepL(key))
    return "Ключ сохранён"
})

w.Bind("goTranslate", func(text string) string {
    if v.translator == nil { return translate.Message(translate.ErrNoKey) }
    if err := translate.Precheck(text); err != nil { return translate.Message(err) }
    v.transSeq++
    seq, tr := v.transSeq, v.translator
    go func() {
        out, err := tr.Translate(context.Background(), text)
        payload, _ := json.Marshal(result{OK: err == nil, Text: out, Msg: translate.Message(err)})
        v.w.Dispatch(func() {
            if seq != v.transSeq { return } // superseded, or the page has changed
            v.w.Eval("mdviewTranslateDone(" + string(payload) + ")")
        })
    }()
    return ""
})
```

The cheap checks stay synchronous and their message goes back through the
existing promise as a toast. The request runs on a goroutine. The result returns
through `Dispatch` + `Eval`, the same route `Reload` already uses.

**`transSeq` needs no mutex.** It is written in `goTranslate` and read inside a
`Dispatch` closure, and both run on the UI thread. The goroutine only reads its
own captured copy. `v.translator` is likewise only ever touched from bindings,
which is why the goroutine captures `tr` up front rather than dereferencing the
field later.

`show` (`viewer.go:239`) also increments `transSeq`, inside its existing
`Dispatch` closure next to `navigateHTML` (`viewer.go:250-253`) so the increment
lands on the UI thread. This is what discards a reply that arrives after the
user has navigated away: without it, a stale translation would be evaluated
against a freshly loaded page.

Splicing JSON into an `Eval` string is safe because `encoding/json` escapes
U+2028 and U+2029, the two characters that are legal in JSON strings but not in
JavaScript string literals.

### `src/internal/renderer/assets/template.html` (modified)

CSS inline in the existing `<style>` block, per `src/internal/renderer/AGENTS.md`.
The popover and the button are written against `--box-bg`, `--box-border`,
`--fg` and `--link`, all of which every one of the seven `body[data-theme=…]`
blocks already defines, so no theme block is touched.

Markup: a `#translate-btn` button and a `#translate-pop` panel, both
`position: fixed`, both hidden by default. The popover is capped at
`max-width: 420px` and `max-height: 40vh` with `overflow: auto`, and carries a
footer reading `DeepL` and `Esc`.

JavaScript:

- A `selectionchange` listener, debounced ~150 ms, shows `#translate-btn` under
  the right edge of the last rect from `getSelection().getRangeAt(0).getClientRects()`
  when the selection is non-empty and inside `.content`, and hides it otherwise.
- The button listens on **`mousedown` with `preventDefault()`**. A `click`
  listener would fire after the selection had already collapsed, leaving nothing
  to translate.
- `T` joins the `keydown` handler beside `e`/`w`/`d`, below the `INPUT` /
  `SELECT` / `TEXTAREA` guard at `template.html:530` so that typing a key
  containing the letter into the settings field does not trigger a translation.
- `mdviewTranslateDone(res)` fills the popover, but only while it is open and in
  its loading state. This is the dismissal case the Go-side sequence number
  cannot see: Esc closes the popover in JS without Go ever hearing about it.
- The popover is registered as an overlay so `Escape` closes it rather than
  exiting. It also closes on click outside and on a new selection.

The settings overlay gains a `Перевод` group matching the existing groups
(`template.html:308-324`): an `<input type="password">`, a status line, and a
save button.

**The key is never sent to the page.** The input renders empty on every load;
the status line is filled from `goHasTranslateKey()`, which returns only a
boolean. Nothing else would do — the settings markup is part of the rendered
document, so a pre-filled value would sit in `%TEMP%` and would be written into
every file saved with Ctrl+S.

The help overlay (`template.html:290-306`) gains a row: `T` — `Translate selection`.

## Error handling

| Situation | Where | Message |
|---|---|---|
| No key configured | Toast | `Ключ DeepL не настроен — задайте его в настройках (S)` |
| Empty selection | — | Nothing happens |
| Over `MaxChars` | Toast | `Слишком длинный фрагмент: N символов, максимум 5000` |
| 403 | Popover | `DeepL отклонил ключ` |
| 456 | Popover | `Исчерпана месячная квота DeepL` |
| 429, 529 | Popover | `Слишком много запросов, попробуйте позже` |
| 413 | Popover | `Слишком длинный фрагмент` |
| 500, 504 | Popover | `DeepL временно недоступен` |
| Timeout, DNS, offline | Popover | `Нет связи с DeepL` |

Every case is distinguishable, because "error" alone would not tell the user
whether to fix the key, wait, or select less.

## Testing

`src/internal/translate/deepl_test.go`, against `httptest.Server`:

- Request shaping: method, path, `Authorization` header, and a body whose `text`
  is an array with `target_lang: "RU"`.
- Response parsing, including a payload with more than one translation (only the
  first is read).
- Each status in the table above maps to its error, checked with `errors.Is`.
- A 400 body carrying `message` surfaces that text in the wrapped error.
- Host selection: a key ending `:fx` targets `api-free`, one without targets
  `api`. Tested by exposing the resolved endpoint on the struct rather than by
  making a request.

`src/internal/translate/translate_test.go`:

- `Precheck` on empty, whitespace-only, exactly `MaxChars`, and `MaxChars+1`.
- `Precheck` counts runes, not bytes: 3000 Cyrillic characters pass despite
  exceeding 5000 bytes.
- `Message` returns a non-empty string for every exported error, a non-empty
  fallback for an unrecognised error, and `""` for `nil` — the success path
  marshals `Message(nil)` into the payload's `msg` field.

`src/internal/translate/cache_test.go`:

- A second call with identical text does not reach the underlying translator.
- Different text does.
- An error is not cached — the next call retries.
- Concurrent calls from several goroutines, so `-race` covers the mutex.

Nothing in `viewer` becomes newly testable: its tests only exercise pure
functions and never construct a `Viewer`, since that would open a window. That
is the reason `MaxChars`, `Precheck` and `Message` live in `translate` rather
than inline in the binding.

Verified by hand: popover placement near the viewport edges, `Esc` closing the
popover rather than the application, the button surviving `mousedown`, and
behaviour across the light and dark themes.

## Privacy

The selected text is sent to DeepL. On the Free tier DeepL's terms permit using
submitted text to improve their models; the Pro tier excludes this. The feature
is inert until a key is configured, so nothing leaves the machine by default.
Moving to Pro is a new key and nothing else — the client already switches hosts
on the `:fx` suffix.

## Out of scope

- Choosing the target language, or any direction other than "into Russian".
- Translating a whole document, or replacing the original text in place.
- A history of past translations, or persisting the cache across runs.
- Retry with backoff on 429 and 500. The message tells the user to retry; an
  automatic retry inside a 10-second timeout would mostly just delay the error.
- Any provider other than DeepL. The `Translator` interface is the single place
  a second one would attach.
