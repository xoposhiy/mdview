# Selection Translation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Select text in mdview, press `T` or click a button, and see the Russian translation in a popover.

**Architecture:** A new stdlib-only `translate` package holds a `Translator` interface, a DeepL v2 client, a memoising wrapper, and every string the user will read. The viewer exposes three bindings; because bound functions run on the WebView2 UI thread, `goTranslate` validates synchronously, runs the request on a goroutine, and returns the result through `Dispatch` + `Eval`. The page owns selection tracking, the popover, and dismissal.

**Tech Stack:** Go 1.22.0, `net/http`, `encoding/json`, `httptest`; vanilla JS and inline CSS in `template.html`.

**Spec:** `docs/superpowers/specs/2026-07-31-selection-translate-design.md`

## Global Constraints

- Go 1.22.0. The `translate` package uses **stdlib only** — no new dependencies, so `src/vendor/` is untouched and `go mod tidy` must not be run.
- All CSS goes **inline in `template.html`**, inside the existing `<style>` block. External stylesheets silently fail in WebView2 (`src/internal/renderer/AGENTS.md`).
- The DeepL API key must **never** reach the page. Bindings expose a boolean status and a setter; nothing returns the key.
- User-facing strings are Russian. Code, comments, test names and this plan are English, matching the repo.
- Build from `src`: `go build -ldflags="-H windowsgui" -o ../mdview.exe .`
- Test from `src`: `go test ./...`
- Windows-only. Close a running `mdview.exe` before rebuilding — the linker cannot overwrite a running binary and leaves a `mdview.exe~` behind.
- Commit messages: imperative, capitalised, no `feat:` prefix, matching `git log`. Every commit ends with:
  `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`

---

### Task 1: `translate` package foundation

The interface, the sentinel errors, the length cap and the user-facing strings. Everything here is pure, which is the whole point: `viewer` tests never construct a `Viewer` (that would open a window), so any logic worth testing must live in this package.

**Files:**
- Create: `src/internal/translate/translate.go`
- Test: `src/internal/translate/translate_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Translator interface { Translate(ctx context.Context, text string) (string, error) }`
  - `const MaxChars = 5000`
  - `var ErrNoKey, ErrEmpty, ErrAuth, ErrQuota, ErrRateLimit, ErrTooLong, ErrTransient error`
  - `type TooLongError struct { Chars int }` with `Error() string` and `Is(error) bool`
  - `func Precheck(text string) error`
  - `func Message(err error) string`

- [ ] **Step 1: Write the failing tests**

Create `src/internal/translate/translate_test.go`:

```go
package translate

import (
	"errors"
	"strings"
	"testing"
)

func TestPrecheckRejectsEmptyAndWhitespace(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t "} {
		if err := Precheck(in); !errors.Is(err, ErrEmpty) {
			t.Errorf("Precheck(%q) = %v, want ErrEmpty", in, err)
		}
	}
}

func TestPrecheckAcceptsTextUpToMaxChars(t *testing.T) {
	if err := Precheck(strings.Repeat("a", MaxChars)); err != nil {
		t.Errorf("Precheck(MaxChars) = %v, want nil", err)
	}
}

func TestPrecheckRejectsTextOverMaxCharsAndReportsCount(t *testing.T) {
	err := Precheck(strings.Repeat("a", MaxChars+1))
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("Precheck(MaxChars+1) = %v, want ErrTooLong", err)
	}
	var tl *TooLongError
	if !errors.As(err, &tl) {
		t.Fatalf("Precheck(MaxChars+1) = %v, want a *TooLongError", err)
	}
	if tl.Chars != MaxChars+1 {
		t.Errorf("Chars = %d, want %d", tl.Chars, MaxChars+1)
	}
}

// Cyrillic is two bytes per character in UTF-8. Counting bytes would reject
// text the user would call well under the limit.
func TestPrecheckCountsRunesNotBytes(t *testing.T) {
	text := strings.Repeat("я", 3000) // 6000 bytes, 3000 runes
	if err := Precheck(text); err != nil {
		t.Errorf("Precheck(3000 Cyrillic runes) = %v, want nil", err)
	}
}

func TestMessageIsEmptyForNil(t *testing.T) {
	if got := Message(nil); got != "" {
		t.Errorf("Message(nil) = %q, want empty", got)
	}
}

func TestMessageIsNonEmptyForEverySentinel(t *testing.T) {
	for _, err := range []error{ErrNoKey, ErrEmpty, ErrAuth, ErrQuota, ErrRateLimit, ErrTooLong, ErrTransient} {
		if got := Message(err); got == "" {
			t.Errorf("Message(%v) is empty", err)
		}
	}
}

func TestMessageFallsBackForUnknownError(t *testing.T) {
	if got := Message(errors.New("dial tcp: no such host")); got == "" {
		t.Error("Message(unknown) is empty, want a fallback")
	}
}

func TestMessageForTooLongNamesTheActualLength(t *testing.T) {
	got := Message(&TooLongError{Chars: 7321})
	if !strings.Contains(got, "7321") {
		t.Errorf("Message(TooLongError{7321}) = %q, want it to name 7321", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd src && go test ./internal/translate/ -v`
Expected: build failure — `undefined: Precheck`, `undefined: ErrEmpty`, and so on.

- [ ] **Step 3: Write the implementation**

Create `src/internal/translate/translate.go`:

```go
// Package translate turns selected text into Russian.
package translate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxChars caps one translation. It is a readability limit for the popover,
// not an API one — DeepL accepts 128 KiB — and its real job is to make an
// accidental Ctrl+A cost nothing.
const MaxChars = 5000

// Translator turns text into Russian. DeepL is the only implementation; the
// interface exists so the viewer never names a concrete client and tests never
// touch the network.
type Translator interface {
	Translate(ctx context.Context, text string) (string, error)
}

var (
	ErrNoKey     = errors.New("translate: no api key")
	ErrEmpty     = errors.New("translate: empty text")
	ErrAuth      = errors.New("translate: auth rejected")
	ErrQuota     = errors.New("translate: quota exceeded")
	ErrRateLimit = errors.New("translate: rate limited")
	ErrTooLong   = errors.New("translate: text too long")
	ErrTransient = errors.New("translate: service unavailable")
)

// TooLongError carries the measured length so the message can name it. DeepL's
// own 413 arrives without a count and uses the bare ErrTooLong sentinel.
type TooLongError struct{ Chars int }

func (e *TooLongError) Error() string { return fmt.Sprintf("translate: text too long: %d chars", e.Chars) }

// Is makes errors.Is(err, ErrTooLong) true for this type, so callers can treat
// both forms alike without caring which produced the failure.
func (e *TooLongError) Is(target error) bool { return target == ErrTooLong }

// Precheck rejects text not worth spending a request on. It guards the package
// boundary; the page checks the selection before ever calling.
func Precheck(text string) error {
	if strings.TrimSpace(text) == "" {
		return ErrEmpty
	}
	if n := utf8.RuneCountInString(text); n > MaxChars {
		return &TooLongError{Chars: n}
	}
	return nil
}

// Message renders err for the popover. It returns "" for nil, which is what
// the success path puts in the payload's msg field.
func Message(err error) string {
	if err == nil {
		return ""
	}
	var tl *TooLongError
	if errors.As(err, &tl) {
		return fmt.Sprintf("Слишком длинный фрагмент: %d символов, максимум %d", tl.Chars, MaxChars)
	}
	switch {
	case errors.Is(err, ErrNoKey):
		return "Ключ DeepL не настроен — задайте его в настройках (S)"
	case errors.Is(err, ErrEmpty):
		return "Ничего не выделено"
	case errors.Is(err, ErrTooLong):
		return fmt.Sprintf("Слишком длинный фрагмент, максимум %d символов", MaxChars)
	case errors.Is(err, ErrAuth):
		return "DeepL отклонил ключ"
	case errors.Is(err, ErrQuota):
		return "Исчерпана месячная квота DeepL"
	case errors.Is(err, ErrRateLimit):
		return "Слишком много запросов, попробуйте позже"
	case errors.Is(err, ErrTransient):
		return "DeepL временно недоступен"
	}
	// Timeouts, DNS failures, a dead link: all indistinguishable to the user,
	// and all mean the same thing.
	return "Нет связи с DeepL"
}
```

The `context` import is used by the `Translator` interface. Leave it.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd src && go test ./internal/translate/ -v`
Expected: PASS, 8 tests.

- [ ] **Step 5: Check the whole tree still builds and passes**

Run: `cd src && go vet ./internal/translate/ && go test ./...`
Expected: no vet output, all packages ok.

- [ ] **Step 6: Commit**

```bash
cd c:/work/mdview
git add src/internal/translate/translate.go src/internal/translate/translate_test.go
git commit -m "Add translate package foundation: errors, length cap, messages

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: DeepL client

**Files:**
- Create: `src/internal/translate/deepl.go`
- Test: `src/internal/translate/deepl_test.go`

**Interfaces:**
- Consumes: `Translator`, `ErrAuth`, `ErrQuota`, `ErrRateLimit`, `ErrTooLong`, `ErrTransient` from Task 1.
- Produces:
  - `type DeepL struct` with unexported fields `key`, `endpoint`, `client`
  - `func NewDeepL(key string) *DeepL`
  - `func (d *DeepL) Endpoint() string`
  - `func (d *DeepL) Translate(ctx context.Context, text string) (string, error)`

Verified against `developers.deepl.com` on 2026-07-31: `text` in the JSON body is an **array** of strings, not a string. Free keys carry the `:fx` suffix and use a different host.

- [ ] **Step 1: Write the failing tests**

Create `src/internal/translate/deepl_test.go`:

```go
package translate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestDeepL points a client at srv instead of the real API. The test lives
// in the same package, so it can set the unexported endpoint directly.
func newTestDeepL(srv *httptest.Server, key string) *DeepL {
	d := NewDeepL(key)
	d.endpoint = srv.URL + "/v2/translate"
	return d
}

func TestDeepLSendsCorrectRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotType string
	var gotBody deeplRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"translations":[{"detected_source_language":"EN","text":"привет"}]}`))
	}))
	defer srv.Close()

	if _, err := newTestDeepL(srv, "abc:fx").Translate(context.Background(), "hello"); err != nil {
		t.Fatalf("Translate returned %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v2/translate" {
		t.Errorf("path = %q, want /v2/translate", gotPath)
	}
	if gotAuth != "DeepL-Auth-Key abc:fx" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotType)
	}
	// text is an array in the v2 API, not a bare string.
	if len(gotBody.Text) != 1 || gotBody.Text[0] != "hello" {
		t.Errorf("body text = %#v, want [\"hello\"]", gotBody.Text)
	}
	if gotBody.TargetLang != "RU" {
		t.Errorf("target_lang = %q, want RU", gotBody.TargetLang)
	}
}

func TestDeepLParsesTranslation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"translations":[{"detected_source_language":"EN","text":"привет мир"}]}`))
	}))
	defer srv.Close()

	got, err := newTestDeepL(srv, "k:fx").Translate(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Translate returned %v", err)
	}
	if got != "привет мир" {
		t.Errorf("got %q, want %q", got, "привет мир")
	}
}

func TestDeepLReadsOnlyTheFirstTranslation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"translations":[{"text":"первый"},{"text":"второй"}]}`))
	}))
	defer srv.Close()

	got, _ := newTestDeepL(srv, "k:fx").Translate(context.Background(), "x")
	if got != "первый" {
		t.Errorf("got %q, want %q", got, "первый")
	}
}

func TestDeepLMapsStatusCodesToErrors(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{403, ErrAuth},
		{413, ErrTooLong},
		{429, ErrRateLimit},
		{529, ErrRateLimit},
		{456, ErrQuota},
		{500, ErrTransient},
		{504, ErrTransient},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
		}))
		_, err := newTestDeepL(srv, "k:fx").Translate(context.Background(), "x")
		if !errors.Is(err, c.want) {
			t.Errorf("status %d gave %v, want %v", c.status, err, c.want)
		}
		srv.Close()
	}
}

func TestDeepLSurfacesMessageForUnmappedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"message":"Value for 'target_lang' not supported."}`))
	}))
	defer srv.Close()

	_, err := newTestDeepL(srv, "k:fx").Translate(context.Background(), "x")
	if err == nil {
		t.Fatal("want an error for status 400")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error = %v, want it to carry DeepL's message", err)
	}
}

func TestDeepLErrorsOnEmptyTranslationsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"translations":[]}`))
	}))
	defer srv.Close()

	if _, err := newTestDeepL(srv, "k:fx").Translate(context.Background(), "x"); err == nil {
		t.Error("want an error for an empty translations array")
	}
}

func TestNewDeepLPicksHostFromKeySuffix(t *testing.T) {
	if got := NewDeepL("279a2e9d-83b3-c416-7e2d-f721593e42a0:fx").Endpoint(); got != "https://api-free.deepl.com/v2/translate" {
		t.Errorf("free key endpoint = %q", got)
	}
	if got := NewDeepL("279a2e9d-83b3-c416-7e2d-f721593e42a0").Endpoint(); got != "https://api.deepl.com/v2/translate" {
		t.Errorf("pro key endpoint = %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd src && go test ./internal/translate/ -run DeepL -v`
Expected: build failure — `undefined: NewDeepL`, `undefined: deeplRequest`.

- [ ] **Step 3: Write the implementation**

Create `src/internal/translate/deepl.go`:

```go
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	freeHost = "https://api-free.deepl.com"
	proHost  = "https://api.deepl.com"

	// DeepL accepts 128 KiB per request; Precheck keeps us far below that. The
	// cap here only bounds how much of an error body we will read.
	maxRespBytes = 1 << 20
	reqTimeout   = 10 * time.Second
)

// DeepL translates through the DeepL API v2.
type DeepL struct {
	key      string
	endpoint string
	client   *http.Client
}

// NewDeepL builds a client for key. Free keys carry the ":fx" suffix and use a
// different host from Pro keys, so the endpoint follows the key rather than a
// separate setting: upgrading to Pro is then just pasting the new key, instead
// of a puzzling 403 against the wrong host.
func NewDeepL(key string) *DeepL {
	host := proHost
	if strings.HasSuffix(key, ":fx") {
		host = freeHost
	}
	return &DeepL{
		key:      key,
		endpoint: host + "/v2/translate",
		// http.Client's zero value has no timeout at all, which would leave the
		// popover spinning forever against a dead connection.
		client: &http.Client{Timeout: reqTimeout},
	}
}

// Endpoint reports the resolved URL.
func (d *DeepL) Endpoint() string { return d.endpoint }

type deeplRequest struct {
	Text       []string `json:"text"`
	TargetLang string   `json:"target_lang"`
}

type deeplResponse struct {
	Translations []struct {
		Text string `json:"text"`
	} `json:"translations"`
	Message string `json:"message"`
}

// Translate sends text to DeepL with target_lang=RU. The source language is
// omitted so DeepL detects it.
func (d *DeepL) Translate(ctx context.Context, text string) (string, error) {
	body, err := json.Marshal(deeplRequest{Text: []string{text}, TargetLang: "RU"})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+d.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return "", err
	}

	// A non-JSON body is not fatal: on an error status the status code alone
	// decides, and parsed.Message simply stays empty.
	var parsed deeplResponse
	json.Unmarshal(raw, &parsed)

	if resp.StatusCode != http.StatusOK {
		return "", statusError(resp.StatusCode, parsed.Message)
	}
	if len(parsed.Translations) == 0 {
		return "", fmt.Errorf("translate: deepl returned no translations")
	}
	return parsed.Translations[0].Text, nil
}

// statusError maps DeepL's documented codes onto the package sentinels. For
// anything unmapped the caller gets DeepL's own message, since there is no
// canned Russian string that would be more useful.
func statusError(code int, msg string) error {
	switch code {
	case 403:
		return ErrAuth
	case 413:
		return ErrTooLong
	case 429, 529:
		return ErrRateLimit
	case 456:
		return ErrQuota
	case 500, 504:
		return ErrTransient
	}
	if msg != "" {
		return fmt.Errorf("translate: deepl %d: %s", code, msg)
	}
	return fmt.Errorf("translate: deepl %d", code)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd src && go test ./internal/translate/ -v`
Expected: PASS, all tests from Tasks 1 and 2.

- [ ] **Step 5: Commit**

```bash
cd c:/work/mdview
git add src/internal/translate/deepl.go src/internal/translate/deepl_test.go
git commit -m "Add DeepL v2 client with status-code error mapping

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Memoising wrapper

**Files:**
- Create: `src/internal/translate/cache.go`
- Test: `src/internal/translate/cache_test.go`

**Interfaces:**
- Consumes: `Translator` from Task 1.
- Produces: `func Cached(t Translator) Translator`

- [ ] **Step 1: Write the failing tests**

Create `src/internal/translate/cache_test.go`:

```go
package translate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeTranslator counts calls and returns whatever it is told to.
type fakeTranslator struct {
	calls atomic.Int32
	out   string
	err   error
}

func (f *fakeTranslator) Translate(ctx context.Context, text string) (string, error) {
	f.calls.Add(1)
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

func TestCacheServesRepeatedTextWithoutCallingThrough(t *testing.T) {
	f := &fakeTranslator{out: "привет"}
	c := Cached(f)

	for i := 0; i < 3; i++ {
		got, err := c.Translate(context.Background(), "hello")
		if err != nil {
			t.Fatalf("call %d returned %v", i, err)
		}
		if got != "привет" {
			t.Fatalf("call %d = %q, want %q", i, got, "привет")
		}
	}
	if n := f.calls.Load(); n != 1 {
		t.Errorf("underlying translator called %d times, want 1", n)
	}
}

func TestCacheCallsThroughForDifferentText(t *testing.T) {
	f := &fakeTranslator{out: "x"}
	c := Cached(f)

	c.Translate(context.Background(), "one")
	c.Translate(context.Background(), "two")

	if n := f.calls.Load(); n != 2 {
		t.Errorf("underlying translator called %d times, want 2", n)
	}
}

// A failure caused by a bad key or an exhausted quota must not be remembered:
// the user fixes the key and retries the same passage.
func TestCacheDoesNotCacheErrors(t *testing.T) {
	f := &fakeTranslator{err: errors.New("boom")}
	c := Cached(f)

	c.Translate(context.Background(), "hello")
	c.Translate(context.Background(), "hello")

	if n := f.calls.Load(); n != 2 {
		t.Errorf("underlying translator called %d times, want 2", n)
	}
}

// Lookups happen on request goroutines, not the UI thread. Run with -race.
func TestCacheIsSafeForConcurrentUse(t *testing.T) {
	f := &fakeTranslator{out: "ok"}
	c := Cached(f)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Translate(context.Background(), string(rune('a'+i%5))); err != nil {
				t.Errorf("Translate returned %v", err)
			}
		}(i)
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd src && go test ./internal/translate/ -run Cache -v`
Expected: build failure — `undefined: Cached`.

- [ ] **Step 3: Write the implementation**

Create `src/internal/translate/cache.go`:

```go
package translate

import (
	"context"
	"sync"
)

type cached struct {
	inner Translator
	mu    sync.Mutex
	m     map[string]string
}

// Cached memoises successful translations for the life of the process, so
// re-selecting a passage costs neither a request nor quota.
//
// Errors are not cached: a failure from a bad key or an exhausted quota should
// retry once the user fixes it. There is no eviction — the bound is how many
// distinct passages one person selects in one sitting.
func Cached(t Translator) Translator {
	return &cached{inner: t, m: make(map[string]string)}
}

func (c *cached) Translate(ctx context.Context, text string) (string, error) {
	c.mu.Lock()
	hit, ok := c.m[text]
	c.mu.Unlock()
	if ok {
		return hit, nil
	}

	// Deliberately unlocked across the request: holding the mutex would
	// serialise every translation. Two concurrent requests for the same text
	// both go out, which costs one extra call in a case a human cannot trigger.
	out, err := c.inner.Translate(ctx, text)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.m[text] = out
	c.mu.Unlock()
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass, including under the race detector**

Run: `cd src && go test ./internal/translate/ -race -v`
Expected: PASS, no race reports.

- [ ] **Step 5: Commit**

```bash
cd c:/work/mdview
git add src/internal/translate/cache.go src/internal/translate/cache_test.go
git commit -m "Add memoising Translator wrapper

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Config field and viewer bindings

Wires the package into the viewer. The one piece of viewer logic that can be tested — building the JSON payload — is extracted into a free function; everything else is glue that needs a live window.

**Files:**
- Modify: `src/internal/config/config.go:9-13`
- Create: `src/internal/viewer/translate.go`
- Test: `src/internal/viewer/translate_test.go`
- Modify: `src/internal/viewer/viewer.go` (imports, struct at `:106-114`, bindings in `New`, `show` at `:239-255`)

**Interfaces:**
- Consumes: `translate.Translator`, `translate.Cached`, `translate.NewDeepL`, `translate.Precheck`, `translate.Message`, `translate.ErrNoKey` from Tasks 1–3.
- Produces, for the page in Tasks 5 and 6:
  - `goHasTranslateKey() → bool`
  - `goSaveTranslateKey(key string) → string` (a toast message)
  - `goTranslate(text string) → string` (`""` when accepted, otherwise a toast message)
  - the page must define `mdviewTranslateDone(res)` where `res` is `{ok: bool, text: string, msg: string}`

- [ ] **Step 1: Write the failing test**

Create `src/internal/viewer/translate_test.go`:

```go
package viewer

import (
	"errors"
	"strings"
	"testing"

	"mdview/internal/translate"
)

func TestTranslatePayloadOnSuccess(t *testing.T) {
	got := translatePayload("привет", nil)
	want := `{"ok":true,"text":"привет","msg":""}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTranslatePayloadOnError(t *testing.T) {
	got := translatePayload("", translate.ErrQuota)
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("got %s, want ok:false", got)
	}
	if !strings.Contains(got, translate.Message(translate.ErrQuota)) {
		t.Errorf("got %s, want it to carry the quota message", got)
	}
}

// The payload is spliced straight into an Eval string, so anything that could
// terminate a JavaScript expression must come back escaped.
func TestTranslatePayloadEscapesDangerousCharacters(t *testing.T) {
	got := translatePayload("a\"b</script>\u2028c", nil)
	for _, bad := range []string{`"b<`, "</script>", "\u2028"} {
		if strings.Contains(got, bad) {
			t.Errorf("payload %s still contains raw %q", got, bad)
		}
	}
}

func TestTranslatePayloadIsValidForUnknownErrors(t *testing.T) {
	got := translatePayload("", errors.New("dial tcp: no such host"))
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("got %s, want ok:false", got)
	}
	if strings.Contains(got, `"msg":""`) {
		t.Errorf("got %s, want a non-empty msg", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src && go test ./internal/viewer/ -run TranslatePayload -v`
Expected: build failure — `undefined: translatePayload`.

- [ ] **Step 3: Add the config field**

In `src/internal/config/config.go`, extend the struct:

```go
type Config struct {
	Theme    string `json:"theme"`    // theme id, e.g. "light", "dark", "dracula"
	Font     string `json:"font"`     // font id, e.g. "sans", "serif", "reading", "mono"
	FontSize int    `json:"fontSize"` // body font size in px
	DeepLKey string `json:"deeplKey"` // DeepL API key; empty disables translation
}
```

`Default()` is left alone — the zero value is the correct default. `Load` already unmarshals over defaults (`config.go:38-40`), so existing `config.json` files without the field keep working.

- [ ] **Step 4: Write the payload helper**

Create `src/internal/viewer/translate.go`:

```go
package viewer

import (
	"encoding/json"

	"mdview/internal/translate"
)

// transResult is what the page receives in mdviewTranslateDone.
type transResult struct {
	OK   bool   `json:"ok"`
	Text string `json:"text"`
	Msg  string `json:"msg"`
}

// translatePayload renders a finished translation as a JSON literal to splice
// into an Eval call. encoding/json escapes U+2028 and U+2029 — legal in JSON
// strings but not in JavaScript ones — plus < > and &, which is what makes the
// result safe to concatenate into JavaScript source.
func translatePayload(text string, err error) string {
	b, mErr := json.Marshal(transResult{OK: err == nil, Text: text, Msg: translate.Message(err)})
	if mErr != nil {
		return `{"ok":false,"text":"","msg":"Не удалось разобрать ответ"}`
	}
	return string(b)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd src && go test ./internal/viewer/ -run TranslatePayload -v`
Expected: PASS, 4 tests.

Note: `TestTranslatePayloadOnSuccess` expects Cyrillic unescaped. `encoding/json` emits UTF-8 for non-ASCII, escaping only `<`, `>`, `&`, U+2028 and U+2029 — so this passes as written.

- [ ] **Step 6: Add the fields and bindings to the viewer**

In `src/internal/viewer/viewer.go`, add to the import block:

```go
"context"
"encoding/json"

"mdview/internal/translate"
```

`encoding/json` is not yet imported by `viewer.go`; `translate.go` in the same package imports it separately, which is fine.

Extend the struct at `viewer.go:106-114`:

```go
type Viewer struct {
	w        webview.WebView
	filePath string
	prefs    config.Config
	scrollY  int
	history  history // documents visited before the current one
	tmpHTML  string  // temp file used for Navigate instead of SetHtml
	renderFn func([]byte, config.Config, int, string) (string, error)

	// translator is nil until a DeepL key is configured. transSeq discards
	// replies that arrive after a newer request or after a navigation; both are
	// only ever touched on the UI thread, so neither needs a mutex.
	translator translate.Translator
	transSeq   int
}
```

In `New`, right after `v := &Viewer{...}` (`viewer.go:148`):

```go
if prefs.DeepLKey != "" {
	v.translator = translate.Cached(translate.NewDeepL(prefs.DeepLKey))
}
```

Add the three bindings next to the other preference bindings (after `goSaveFontSize`, `viewer.go:216-219`):

```go
w.Bind("goHasTranslateKey", func() bool { return v.translator != nil })

w.Bind("goSaveTranslateKey", func(key string) string {
	key = strings.TrimSpace(key)
	v.prefs.DeepLKey = key
	config.Save(v.prefs)
	if key == "" {
		v.translator = nil
		return "Ключ удалён"
	}
	// A fresh cache: the old one belongs to the old key.
	v.translator = translate.Cached(translate.NewDeepL(key))
	return "Ключ сохранён"
})

// goTranslate returns "" once the request is accepted; any other string is a
// message for the page to toast. The request itself cannot run here: bound
// functions execute on the UI thread (see msgcb in go-webview2), so a blocking
// HTTP call would freeze the window for its whole duration.
w.Bind("goTranslate", func(text string) string {
	if v.translator == nil {
		return translate.Message(translate.ErrNoKey)
	}
	if err := translate.Precheck(text); err != nil {
		return translate.Message(err)
	}

	v.transSeq++
	seq, tr := v.transSeq, v.translator

	go func() {
		out, err := tr.Translate(context.Background(), text)
		if err != nil {
			applog.Write("goTranslate: %v", err)
		}
		payload := translatePayload(out, err)
		v.w.Dispatch(func() {
			// Dispatch runs on the UI thread, the only place transSeq changes,
			// so this comparison is race-free. A mismatch means the user asked
			// again or navigated away while this request was in flight.
			if seq != v.transSeq {
				return
			}
			v.w.Eval("mdviewTranslateDone(" + payload + ")")
		})
	}()
	return ""
})
```

In `show` (`viewer.go:250-253`), bump the counter inside the existing `Dispatch` closure so the increment lands on the UI thread:

```go
v.w.Dispatch(func() {
	// A new document invalidates any translation still in flight.
	v.transSeq++
	v.w.SetTitle(TitleFromPath(path))
	v.navigateHTML(html)
})
```

- [ ] **Step 7: Verify the tree builds and every test still passes**

Run: `cd src && go vet ./... && go test ./...`
Expected: no vet output; all packages ok.

Behaviour cannot be exercised yet — nothing calls the bindings until Task 6. Compilation plus the payload tests are the gate here.

- [ ] **Step 8: Commit**

```bash
cd c:/work/mdview
git add src/internal/config/config.go src/internal/viewer/viewer.go src/internal/viewer/translate.go src/internal/viewer/translate_test.go
git commit -m "Add translation bindings to the viewer

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: API key field in the settings panel

**Files:**
- Modify: `src/internal/renderer/assets/template.html` — `<style>` block, settings overlay at `:308-324`, binding fallbacks at `:329-342`, `load` handler at `:482-491`

**Interfaces:**
- Consumes: `goHasTranslateKey`, `goSaveTranslateKey` from Task 4; the existing `showToast(msg)` at `template.html:498`.
- Produces: `refreshKeyStatus()`, called on load and after a save.

- [ ] **Step 1: Add the CSS**

In the `<style>` block, after the `#size-val` rule (`template.html:263`):

```css
#deepl-key {
    width: 100%; box-sizing: border-box;
    padding: 6px 10px; margin-bottom: 9px;
    border: 1px solid var(--box-border); border-radius: 6px;
    background: var(--bg); color: var(--fg);
    font-family: Consolas, 'Cascadia Mono', monospace; font-size: 13px;
}
#deepl-key:focus { outline: none; border-color: var(--link); }
.key-row { display: flex; align-items: center; gap: 10px; }
#deepl-status { font-size: 0.88em; opacity: 0.7; }
```

Every colour comes from a variable that all seven `body[data-theme=…]` blocks already define, so no theme block is touched.

- [ ] **Step 2: Add the markup**

In the settings overlay, after the font-size group (`template.html:319-322`), before the closing `</div>`s:

```html
    <div class="setting-group">
      <div class="setting-label">Перевод</div>
      <input type="password" id="deepl-key" placeholder="ключ DeepL" autocomplete="off" spellcheck="false">
      <div class="key-row">
        <button class="chip" id="deepl-save">Сохранить</button>
        <span id="deepl-status"></span>
      </div>
    </div>
```

The field is rendered empty on every load and cleared after every save. Nothing ever puts the key into it — the settings markup is part of the rendered document, so a pre-filled value would sit in `%TEMP%` and be written into every file saved with Ctrl+S.

- [ ] **Step 3: Add the binding fallbacks**

With the other fallbacks (`template.html:329-342`), so the page still works when opened outside the app:

```javascript
if (typeof goHasTranslateKey === 'undefined')  { goHasTranslateKey  = function() { return Promise.resolve(false); }; }
if (typeof goSaveTranslateKey === 'undefined') { goSaveTranslateKey = function() { return Promise.resolve(''); }; }
if (typeof goTranslate === 'undefined')        { goTranslate        = function() { return Promise.resolve(''); }; }
```

`goTranslate` is declared here too; Task 6 uses it.

- [ ] **Step 4: Add the behaviour**

After `buildSettings()` (`template.html:412`):

```javascript
/* The key never travels to the page: goHasTranslateKey returns only a boolean,
   and the input is emptied the moment its value has been handed to Go. */
function refreshKeyStatus() {
  goHasTranslateKey().then(function(has) {
    document.getElementById('deepl-status').textContent = has ? 'ключ настроен' : 'ключ не настроен';
  });
}
document.getElementById('deepl-save').addEventListener('click', function() {
  var input = document.getElementById('deepl-key');
  goSaveTranslateKey(input.value).then(function(msg) {
    input.value = '';
    refreshKeyStatus();
    toastIfMsg(msg);
  });
});
```

`toastIfMsg` is already defined at `template.html:427`.

In the `load` handler (`template.html:482-491`), add:

```javascript
    refreshKeyStatus();
```

- [ ] **Step 5: Build and verify by hand**

```bash
cd c:/work/mdview/src
go build -ldflags="-H windowsgui" -o ../mdview.exe .
../mdview.exe ../README.md
```

Check, in order:
1. Press `S`. The panel shows a **Перевод** group with an empty password field and `ключ не настроен`.
2. Type anything into the field and click **Сохранить**. A toast says `Ключ сохранён`, the field empties, the status becomes `ключ настроен`.
3. Press `S` twice to reopen. The field is still empty and the status still reads `ключ настроен`.
4. Close and relaunch mdview, press `S`. Status is `ключ настроен` — it survived the restart.
5. Confirm `%APPDATA%\mdview\config.json` contains `deeplKey`, and that the key is **not** anywhere in the page: press Ctrl+S, open the generated `README.html` in an editor, and search for the value you typed. It must not appear.
6. Clear the field and click **Сохранить**. Toast `Ключ удалён`, status `ключ не настроен`.
7. Type into the field and confirm that `s`, `w`, `d`, `e` and `t` insert characters rather than firing shortcuts — the `INPUT` guard at `template.html:530` covers this.

Set a real DeepL key at the end of this check; Task 6 needs one.

- [ ] **Step 6: Commit**

```bash
cd c:/work/mdview
git add src/internal/renderer/assets/template.html
git commit -m "Add DeepL key field to the settings panel

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Selection button, popover and hotkey

**Files:**
- Modify: `src/internal/renderer/assets/template.html` — `<style>` block, body markup near `:286-288`, help table at `:293-304`, `anyOverlayOpen`/`closeOverlays` at `:422-425`, `keydown` handler at `:541`

**Interfaces:**
- Consumes: `goTranslate` from Task 4 (declared in Task 5), `toastIfMsg` at `:427`.
- Produces: `mdviewTranslateDone(res)`, the global the Go side evaluates.

- [ ] **Step 1: Add the CSS**

After the `#deepl-status` rule from Task 5:

```css
/* ----- Translation ----- */
#translate-btn {
    position: fixed; z-index: 1600; display: none;
    width: 26px; height: 26px; border-radius: 50%; padding: 0;
    border: 1px solid var(--box-border); background: var(--box-bg); color: var(--fg);
    font-size: 13px; font-weight: 600; cursor: pointer;
    align-items: center; justify-content: center;
    box-shadow: 0 2px 8px rgba(0,0,0,0.22);
    font-family: 'Segoe UI', system-ui, sans-serif;
}
#translate-btn.visible { display: flex; }
#translate-btn:hover { border-color: var(--link); color: var(--link); }
#translate-pop {
    position: fixed; z-index: 1700; display: none;
    max-width: 420px; max-height: 40vh; overflow: auto;
    padding: 14px 16px; border-radius: 8px;
    background: var(--box-bg); border: 1px solid var(--box-border); color: var(--fg);
    box-shadow: 0 8px 32px rgba(0,0,0,0.28);
    font-size: 15px; line-height: 1.5;
    font-family: 'Segoe UI', system-ui, sans-serif;
}
#translate-pop.visible { display: block; }
#translate-text { white-space: pre-wrap; }
#translate-foot {
    margin-top: 10px; font-size: 0.78em; opacity: 0.55;
    display: flex; justify-content: space-between;
}
```

- [ ] **Step 2: Add the markup**

After the settings button (`template.html:288`):

```html
<button id="translate-btn" title="Перевести (T)">Я</button>
<div id="translate-pop">
  <div id="translate-text"></div>
  <div id="translate-foot"><span>DeepL</span><span>Esc</span></div>
</div>
```

Add a row to the help table, after the `E` row (`template.html:299`):

```html
      <tr><td>T</td><td>Translate selection</td></tr>
```

- [ ] **Step 3: Add the selection tracking and popover**

After the settings behaviour added in Task 5:

```javascript
/* ----- Translation -----
   The Go side owns request sequencing; the page owns the selection, the
   popover and dismissal. transLoading is the piece Go cannot see: Esc closes
   the popover here without Go ever hearing about it, and a reply arriving
   afterwards must be dropped. */
var transLoading = false;
var transAnchor = null;   // selection rect captured when the request started

function selectionText() {
  var sel = window.getSelection();
  if (!sel || sel.isCollapsed) return '';
  var node = sel.anchorNode;
  if (node && node.nodeType === 3) node = node.parentNode;
  if (!node || !node.closest || !node.closest('.content')) return '';
  return sel.toString().trim();
}

function selectionRect() {
  var sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  var rects = sel.getRangeAt(0).getClientRects();
  return rects.length ? rects[rects.length - 1] : null;
}

function placeTransBtn() {
  var btn = document.getElementById('translate-btn');
  var rect = selectionRect();
  if (!selectionText() || !rect) { btn.classList.remove('visible'); return; }
  btn.style.top  = Math.min(rect.bottom + 6, window.innerHeight - 32) + 'px';
  btn.style.left = Math.max(6, Math.min(rect.right - 13, window.innerWidth - 32)) + 'px';
  btn.classList.add('visible');
}

var _selTimer;
document.addEventListener('selectionchange', function() {
  clearTimeout(_selTimer);
  _selTimer = setTimeout(placeTransBtn, 150);
});

function openTransPop(body) {
  var pop = document.getElementById('translate-pop');
  document.getElementById('translate-text').textContent = body;
  pop.classList.add('visible');   // must be displayed before it can be measured
  var r = transAnchor;
  var top  = r ? r.bottom + 10 : 80;
  var left = r ? r.left : 40;
  // Flip above the selection when there is no room below.
  if (top + pop.offsetHeight > window.innerHeight - 10) {
    top = (r ? r.top : 90) - pop.offsetHeight - 10;
  }
  left = Math.min(left, window.innerWidth - pop.offsetWidth - 10);
  pop.style.top  = Math.max(10, top) + 'px';
  pop.style.left = Math.max(10, left) + 'px';
}

function transPopOpen() {
  return document.getElementById('translate-pop').classList.contains('visible');
}

function closeTransPop() {
  document.getElementById('translate-pop').classList.remove('visible');
  transLoading = false;
}

function startTranslate() {
  var text = selectionText();
  if (!text) return;
  transAnchor = selectionRect();
  document.getElementById('translate-btn').classList.remove('visible');
  transLoading = true;
  openTransPop('…');
  goTranslate(text).then(function(msg) {
    // A non-empty reply means Go rejected the request outright; no result is
    // coming, so show the reason in place of the spinner.
    if (msg) { transLoading = false; openTransPop(msg); }
  });
}

// Called from Go via Eval once the request finishes.
function mdviewTranslateDone(res) {
  if (!transLoading) return;   // popover was dismissed while the request ran
  transLoading = false;
  openTransPop(res.ok ? res.text : res.msg);
}

// mousedown, not click: by the time click fires the selection has collapsed
// and there is nothing left to translate.
document.getElementById('translate-btn').addEventListener('mousedown', function(e) {
  e.preventDefault();
  startTranslate();
});

// Dismiss on a click anywhere else. Bubbles after the button's own handler, so
// clicking the button does not immediately close what it just opened.
document.addEventListener('mousedown', function(e) {
  if (!transPopOpen()) return;
  var pop = document.getElementById('translate-pop');
  if (pop.contains(e.target) || e.target.id === 'translate-btn') return;
  closeTransPop();
});
```

- [ ] **Step 4: Make Esc close the popover instead of the application**

Replace `anyOverlayOpen` and `closeOverlays` (`template.html:422-425`):

```javascript
function anyOverlayOpen() {
  return document.querySelector('.overlay.visible') !== null || transPopOpen();
}
function closeOverlays() {
  document.querySelectorAll('.overlay.visible').forEach(function(o) { o.classList.remove('visible'); });
  closeTransPop();
}
```

Without this the popover is invisible to the `Escape` branch at `template.html:506`, and dismissing it would quit mdview.

- [ ] **Step 5: Add the hotkey**

In the `keydown` handler, immediately after the `e`/`E` line (`template.html:541`):

```javascript
    if (e.key === 't' || e.key === 'T') { startTranslate(); return; }
```

This sits below the `INPUT` / `SELECT` / `TEXTAREA` guard at `template.html:530`, so typing a key containing the letter `t` into the settings field does not fire a translation.

- [ ] **Step 6: Build and verify by hand**

```bash
cd c:/work/mdview/src
go build -ldflags="-H windowsgui" -o ../mdview.exe .
../mdview.exe ../README.md
```

With a real DeepL key configured:
1. Select an English sentence. The `Я` button appears just below its right edge.
2. Press `T`. The popover opens showing `…`, then the Russian translation.
3. Select another sentence and click the `Я` button. Same result — the selection survives the click.
4. Press `Esc`. The popover closes and **mdview stays open**.
5. Press `Esc` again with nothing open. mdview exits. Relaunch.
6. Translate the same sentence twice. The second is instant — served from the cache.
7. Select a sentence near the bottom of the window and translate. The popover flips above the selection rather than running off-screen. Repeat near the right edge.
8. Click anywhere in the text. The popover closes.
9. Press `?`. The help table lists `T — Translate selection`.
10. Press `D` to switch to the dark theme and translate again; the popover is legible.
11. Press `T` with nothing selected. Nothing happens.
12. Press Ctrl+A, then `T`. A toast names the character count and the 5000 limit; no request is sent.
13. Start a translation and immediately press `→` to navigate to another file. No popover appears on the new document — the sequence check discarded the reply.

Without a key (clear it in the settings panel first):
14. Select text and press `T`. A toast reads `Ключ DeepL не настроен — задайте его в настройках (S)`, and no popover opens.

Disconnect the network, restore the key, then:
15. Select text and press `T`. After about ten seconds the popover reads `Нет связи с DeepL`. The window stays responsive throughout — scroll and press `S` while waiting to confirm the UI thread is not blocked. This is the check that the async binding actually works; a synchronous implementation would freeze here.

- [ ] **Step 7: Run the full test suite**

Run: `cd src && go test ./... && go vet ./...`
Expected: all packages ok, no vet output.

- [ ] **Step 8: Commit**

```bash
cd c:/work/mdview
git add src/internal/renderer/assets/template.html
git commit -m "Add selection translation: button, hotkey and popover

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Record the decisions in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`, the "Key decisions" list

- [ ] **Step 1: Add the entries**

Append to the `## Key decisions` list:

```markdown
- Translation runs in Go, not the page: the document is served from a temp file, so it has an opaque `file://` origin, and putting the API key in the page would leave it in `%TEMP%`.
- `webview.Bind` callbacks run on the UI thread (`msgcb` in go-webview2), so `goTranslate` validates synchronously and returns immediately; the HTTP request runs on a goroutine and reports back via `Dispatch` + `Eval`. Never block in a binding.
- `Viewer.transSeq` discards translations that arrive after a newer request or a navigation. It needs no mutex: it is only ever touched on the UI thread.
- The DeepL key never reaches the page. `goHasTranslateKey` returns a boolean and the settings input is always empty, so the key stays out of the DOM, out of `%TEMP%`, and out of HTML saved with Ctrl+S.
- `translate.Translator` is an interface with the DeepL client behind it, following `watcher.FileWatcher`. Logic worth testing lives in `translate`, because `viewer` tests never construct a `Viewer` — that would open a window.
```

- [ ] **Step 2: Commit**

```bash
cd c:/work/mdview
git add CLAUDE.md
git commit -m "Document translation decisions in CLAUDE.md

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage**

| Spec section | Task |
|---|---|
| Behaviour table | 4 (bindings), 6 (page) |
| DeepL API — host, header, array body, response | 2 |
| `translate` package: `Translator`, `MaxChars`, `Precheck`, `Message` | 1 |
| `deepl.go`, timeout, status mapping | 2 |
| `cache.go` | 3 |
| `config.go` — `DeepLKey` | 4 |
| `viewer.go` — fields, three bindings, `show` increment | 4 |
| `template.html` — settings key group | 5 |
| `template.html` — button, popover, hotkey, overlay integration, help row | 6 |
| Error handling table | 1 (`Message`), 2 (mapping) |
| Testing section | 1, 2, 3, 4 automated; 5, 6 manual |
| Privacy | no code; the feature is inert without a key (Task 4) |

**Deviation from the spec:** the spec sketched `Precheck` returning the bare `ErrTooLong`, which cannot produce the message "N символов" the behaviour table promises. Task 1 adds `TooLongError{Chars int}` with an `Is` method, so `errors.Is(err, ErrTooLong)` still holds and the count survives to `Message`. DeepL's own 413 keeps the countless sentinel.

**Type consistency:** `Translator`, `MaxChars`, `Precheck`, `Message`, `TooLongError`, `NewDeepL`, `Endpoint`, `Cached`, `deeplRequest`, `deeplResponse`, `statusError`, `transResult`, `translatePayload`, `goHasTranslateKey`, `goSaveTranslateKey`, `goTranslate`, `mdviewTranslateDone`, `transLoading`, `transAnchor`, `selectionText`, `selectionRect`, `placeTransBtn`, `openTransPop`, `transPopOpen`, `closeTransPop`, `startTranslate`, `refreshKeyStatus` — each defined once and used under that name everywhere else.
