// Package translate translates a piece of text through an online service.
// The viewer uses it to translate the selection.
package translate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Result is one translation.
type Result struct {
	Text   string // the translation
	Source string // detected source language, e.g. "EN"
	Target string // the language actually used, e.g. "RU"
}

// Translator translates text into the target language. The service detects
// the source language.
type Translator interface {
	Translate(ctx context.Context, text, target string) (Result, error)
}

// MaxChars is the longest selection mdview sends, in characters (runes). It
// is a readability cap, not an API limit: an accidental Ctrl+A must not spend
// the quota.
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

// TooLongError reports a selection over MaxChars.
type TooLongError struct{ Chars int }

func (e *TooLongError) Error() string {
	return fmt.Sprintf("text too long: %d characters, the limit is %d", e.Chars, MaxChars)
}

func (e *TooLongError) Is(target error) bool { return target == ErrTooLong }

// StatusError is an HTTP status that has no error of its own.
type StatusError struct {
	Code int
	Msg  string // the service's own message, if the body had one
}

func (e *StatusError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("HTTP %d", e.Code)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Msg)
}

// Precheck rejects text that must not be sent: empty, whitespace only, or
// longer than MaxChars.
func Precheck(text string) error {
	if strings.TrimSpace(text) == "" {
		return ErrEmpty
	}
	if n := utf8.RuneCountInString(text); n > MaxChars {
		return &TooLongError{Chars: n}
	}
	return nil
}

// Message returns the English text that the card shows for err, or "" for
// nil. configPath goes into the message for a missing key.
func Message(err error, configPath string) string {
	var tooLong *TooLongError
	var status *StatusError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNoKey):
		return `No DeepL key. Add "translate.deeplKey" to ` + configPath
	case errors.Is(err, ErrEmpty):
		return "Nothing to translate"
	case errors.As(err, &tooLong):
		return fmt.Sprintf("Selection is too long: %d characters, the limit is %d", tooLong.Chars, MaxChars)
	case errors.Is(err, ErrAuth):
		return "DeepL rejected the key"
	case errors.Is(err, ErrQuota):
		return "DeepL monthly quota is used up"
	case errors.Is(err, ErrRateLimit):
		return "Too many requests. Try again later"
	case errors.Is(err, ErrTooLong):
		return "Selection is too long for DeepL"
	case errors.Is(err, ErrTransient):
		return "DeepL is temporarily unavailable"
	case isNetwork(err):
		return "Cannot reach DeepL"
	case errors.As(err, &status):
		if status.Msg != "" {
			return "DeepL error: " + status.Msg
		}
		return fmt.Sprintf("DeepL error: HTTP %d", status.Code)
	default:
		return "DeepL error: " + err.Error()
	}
}

// Retryable reports whether the same request may succeed later.
func Retryable(err error) bool {
	return errors.Is(err, ErrRateLimit) || errors.Is(err, ErrTransient) || isNetwork(err)
}

// isNetwork reports a failure to reach the service: timeout, DNS, offline.
func isNetwork(err error) bool {
	var urlErr *url.Error
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.As(err, &urlErr) || errors.As(err, &netErr)
}

// SameLanguage compares two language codes by their primary part, so "EN-GB"
// and "en" are the same language.
func SameLanguage(a, b string) bool {
	pa, _, _ := strings.Cut(a, "-")
	pb, _, _ := strings.Cut(b, "-")
	return pa != "" && strings.EqualFold(pa, pb)
}

// WithFallback translates text into to. If the detected source language is
// already to, it translates the text again into fallback. An empty fallback,
// or a fallback equal to to, turns the second request off.
func WithFallback(ctx context.Context, t Translator, text, to, fallback string) (Result, error) {
	res, err := t.Translate(ctx, text, to)
	if err != nil {
		return Result{}, err
	}
	if fallback == "" || SameLanguage(fallback, to) || !SameLanguage(res.Source, to) {
		return res, nil
	}
	return t.Translate(ctx, text, fallback)
}
