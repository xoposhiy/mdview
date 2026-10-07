package translate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestDeepL(t *testing.T, h http.HandlerFunc) *DeepL {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	d := NewDeepL("test-key:fx")
	d.endpoint = srv.URL + "/v2/translate"
	return d
}

func TestDeepLRequestShape(t *testing.T) {
	var body map[string]any
	d := newTestDeepL(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v2/translate" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "DeepL-Auth-Key test-key:fx" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &body); err != nil {
			t.Errorf("body is not JSON: %s", data)
		}
		io.WriteString(w, `{"translations":[{"detected_source_language":"EN","text":"привет"}]}`)
	})
	if _, err := d.Translate(context.Background(), "hello", "RU"); err != nil {
		t.Fatal(err)
	}
	texts, ok := body["text"].([]any)
	if !ok || len(texts) != 1 || texts[0] != "hello" {
		t.Errorf("text = %#v, want [\"hello\"]", body["text"])
	}
	if body["target_lang"] != "RU" {
		t.Errorf("target_lang = %#v", body["target_lang"])
	}
	if _, ok := body["source_lang"]; ok {
		t.Error("source_lang is sent")
	}
}

func TestDeepLParsesFirstTranslation(t *testing.T) {
	d := newTestDeepL(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"translations":[
			{"detected_source_language":"DE","text":"first"},
			{"detected_source_language":"FR","text":"second"}]}`)
	})
	res, err := d.Translate(context.Background(), "x", "EN-GB")
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Text: "first", Source: "DE", Target: "EN-GB"}
	if res != want {
		t.Errorf("res = %+v, want %+v", res, want)
	}
}

func TestDeepLEmptyTranslations(t *testing.T) {
	d := newTestDeepL(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"translations":[]}`)
	})
	if _, err := d.Translate(context.Background(), "x", "RU"); err == nil {
		t.Error("want an error for an empty list")
	}
}

func TestDeepLStatusMapping(t *testing.T) {
	cases := map[int]error{
		403: ErrAuth,
		413: ErrTooLong,
		429: ErrRateLimit,
		529: ErrRateLimit,
		456: ErrQuota,
		500: ErrTransient,
		504: ErrTransient,
	}
	for code, want := range cases {
		d := newTestDeepL(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		})
		_, err := d.Translate(context.Background(), "x", "RU")
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", code, err, want)
		}
	}
}

func TestDeepLOtherStatusKeepsMessage(t *testing.T) {
	d := newTestDeepL(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"message":"Value for 'target_lang' not supported."}`)
	})
	_, err := d.Translate(context.Background(), "x", "XX")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 400 || se.Msg != "Value for 'target_lang' not supported." {
		t.Fatalf("err = %#v", err)
	}
	if got := Message(err, ""); got != "DeepL error: Value for 'target_lang' not supported." {
		t.Errorf("Message = %q", got)
	}
}

func TestDeepLOtherStatusWithoutBody(t *testing.T) {
	d := newTestDeepL(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(418)
	})
	_, err := d.Translate(context.Background(), "x", "RU")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 418 {
		t.Fatalf("err = %#v", err)
	}
	if got := Message(err, ""); got != "DeepL error: HTTP 418" {
		t.Errorf("Message = %q", got)
	}
}

func TestDeepLNetworkErrorIsRetryable(t *testing.T) {
	d := NewDeepL("k")
	d.endpoint = "http://127.0.0.1:1/v2/translate" // nothing listens on port 1
	_, err := d.Translate(context.Background(), "x", "RU")
	if err == nil || !Retryable(err) {
		t.Errorf("err = %v, Retryable = %v", err, Retryable(err))
	}
	if got := Message(err, ""); got != "Cannot reach DeepL" {
		t.Errorf("Message = %q", got)
	}
}

func TestDeepLHost(t *testing.T) {
	if got := NewDeepL("abc:fx").endpoint; got != "https://api-free.deepl.com/v2/translate" {
		t.Errorf("free key endpoint = %s", got)
	}
	if got := NewDeepL("abc").endpoint; got != "https://api.deepl.com/v2/translate" {
		t.Errorf("pro key endpoint = %s", got)
	}
}

func TestDeepLHasTimeout(t *testing.T) {
	if NewDeepL("k").client.Timeout == 0 {
		t.Error("client has no timeout")
	}
}
