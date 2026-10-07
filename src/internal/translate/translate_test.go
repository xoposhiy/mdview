package translate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestPrecheck(t *testing.T) {
	cases := []struct {
		name string
		text string
		want error
	}{
		{"empty", "", ErrEmpty},
		{"whitespace", " \t\n ", ErrEmpty},
		{"one word", "hello", nil},
		{"exactly MaxChars", strings.Repeat("a", MaxChars), nil},
		{"MaxChars+1", strings.Repeat("a", MaxChars+1), ErrTooLong},
		{"Cyrillic counts runes", strings.Repeat("я", 3000), nil},
	}
	for _, c := range cases {
		err := Precheck(c.text)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: Precheck = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestPrecheckReportsLength(t *testing.T) {
	err := Precheck(strings.Repeat("я", MaxChars+7))
	var tl *TooLongError
	if !errors.As(err, &tl) || tl.Chars != MaxChars+7 {
		t.Fatalf("err = %v, want TooLongError{%d}", err, MaxChars+7)
	}
	want := fmt.Sprintf("Selection is too long: %d characters, the limit is %d", MaxChars+7, MaxChars)
	if got := Message(err, ""); got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
}

func networkError() error {
	return &url.Error{Op: "Post", URL: "https://x", Err: errors.New("dial tcp: no such host")}
}

func TestMessage(t *testing.T) {
	if got := Message(nil, "p"); got != "" {
		t.Errorf("Message(nil) = %q, want empty", got)
	}
	cases := []struct {
		err  error
		want string
	}{
		{ErrAuth, "DeepL rejected the key"},
		{ErrQuota, "DeepL monthly quota is used up"},
		{ErrRateLimit, "Too many requests. Try again later"},
		{ErrTooLong, "Selection is too long for DeepL"},
		{ErrTransient, "DeepL is temporarily unavailable"},
		{networkError(), "Cannot reach DeepL"},
		{context.DeadlineExceeded, "Cannot reach DeepL"},
		{&StatusError{Code: 400, Msg: "bad lang"}, "DeepL error: bad lang"},
		{fmt.Errorf("wrap: %w", ErrQuota), "DeepL monthly quota is used up"},
	}
	for _, c := range cases {
		if got := Message(c.err, ""); got != c.want {
			t.Errorf("Message(%v) = %q, want %q", c.err, got, c.want)
		}
	}
	for _, err := range []error{ErrEmpty, errors.New("something odd")} {
		if Message(err, "") == "" {
			t.Errorf("Message(%v) is empty", err)
		}
	}
	path := `C:\Users\me\AppData\Roaming\mdview\config.json`
	want := `No DeepL key. Add "translate.deeplKey" to ` + path
	if got := Message(ErrNoKey, path); got != want {
		t.Errorf("Message(ErrNoKey) = %q, want %q", got, want)
	}
}

func TestRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{ErrNoKey, false},
		{ErrEmpty, false},
		{ErrAuth, false},
		{ErrQuota, false},
		{ErrTooLong, false},
		{&TooLongError{Chars: 6000}, false},
		{&StatusError{Code: 400}, false},
		{errors.New("something odd"), false},
		{ErrRateLimit, true},
		{ErrTransient, true},
		{networkError(), true},
		{context.DeadlineExceeded, true},
	}
	for _, c := range cases {
		if got := Retryable(c.err); got != c.want {
			t.Errorf("Retryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestSameLanguage(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"EN", "EN-GB", true},
		{"en", "EN", true},
		{"PT-BR", "pt", true},
		{"RU", "EN", false},
		{"", "EN", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := SameLanguage(c.a, c.b); got != c.want {
			t.Errorf("SameLanguage(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// fake answers with a fixed source language and records the targets.
type fake struct {
	source  string
	targets []string
	errOn   map[string]error // target -> error
}

func (f *fake) Translate(_ context.Context, text, target string) (Result, error) {
	f.targets = append(f.targets, target)
	if err := f.errOn[target]; err != nil {
		return Result{}, err
	}
	return Result{Text: text + "@" + target, Source: f.source, Target: target}, nil
}

func TestWithFallbackDifferentSource(t *testing.T) {
	f := &fake{source: "EN"}
	res, err := WithFallback(context.Background(), f, "hi", "RU", "EN")
	if err != nil || res.Target != "RU" || res.Text != "hi@RU" || len(f.targets) != 1 {
		t.Errorf("res=%+v err=%v targets=%v", res, err, f.targets)
	}
}

func TestWithFallbackSameSource(t *testing.T) {
	f := &fake{source: "RU"}
	res, err := WithFallback(context.Background(), f, "привет", "RU", "EN")
	if err != nil || res.Target != "EN" || res.Source != "RU" || len(f.targets) != 2 {
		t.Errorf("res=%+v err=%v targets=%v", res, err, f.targets)
	}
}

func TestWithFallbackNoSecondCall(t *testing.T) {
	for _, fallback := range []string{"", "ru", "RU"} {
		f := &fake{source: "RU"}
		res, err := WithFallback(context.Background(), f, "x", "RU", fallback)
		if err != nil || res.Target != "RU" || len(f.targets) != 1 {
			t.Errorf("fallback %q: res=%+v err=%v targets=%v", fallback, res, err, f.targets)
		}
	}
}

func TestWithFallbackErrors(t *testing.T) {
	f := &fake{source: "RU", errOn: map[string]error{"EN": ErrQuota}}
	if _, err := WithFallback(context.Background(), f, "x", "RU", "EN"); !errors.Is(err, ErrQuota) {
		t.Errorf("second call: err=%v, want ErrQuota", err)
	}
	f = &fake{source: "EN", errOn: map[string]error{"RU": ErrAuth}}
	_, err := WithFallback(context.Background(), f, "x", "RU", "EN")
	if !errors.Is(err, ErrAuth) || len(f.targets) != 1 {
		t.Errorf("first call: err=%v targets=%v", err, f.targets)
	}
}
