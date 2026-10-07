package viewer

import (
	"context"
	"encoding/json"
	"time"

	"mdview/internal/applog"
	"mdview/internal/config"
	"mdview/internal/translate"
)

// translatePayload is what the page receives for one translation request,
// either from the goTranslate promise or through mdviewTranslateDone.
type translatePayload struct {
	OK     bool   `json:"ok"`
	Text   string `json:"text,omitempty"`
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
	Msg    string `json:"msg,omitempty"`
	Retry  bool   `json:"retry"`
}

// marshalResult turns a translation or an error into the JSON the page reads.
// encoding/json escapes U+2028 and U+2029, so the result is also safe to put
// into a JavaScript source string for Eval.
func marshalResult(res translate.Result, err error) string {
	p := translatePayload{OK: true, Text: res.Text, Source: res.Source, Target: res.Target}
	if err != nil {
		path, _ := config.Path()
		p = translatePayload{Msg: translate.Message(err, path), Retry: translate.Retryable(err)}
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// translatorFor returns the translator for key, and builds a new one (with an
// empty cache) when the key in the file has changed. Runs on the UI thread.
func (v *Viewer) translatorFor(key string) translate.Translator {
	if v.translator == nil || v.translatorKey != key {
		v.translator = translate.Cached(translate.NewDeepL(key))
		v.translatorKey = key
	}
	return v.translator
}

// goTranslate starts a translation of text and returns "" at once. The result
// arrives later through mdviewTranslateDone. A request that cannot start (no
// key, bad text) returns its error payload directly. Runs on the UI thread,
// so it must not wait for the network.
func (v *Viewer) goTranslate(text string) string {
	// Read the file on every request: the user edits the key by hand while
	// mdview runs.
	cfg := config.Load().Translate
	if cfg.DeepLKey == "" {
		return marshalResult(translate.Result{}, translate.ErrNoKey)
	}
	if err := translate.Precheck(text); err != nil {
		return marshalResult(translate.Result{}, err)
	}
	tr := v.translatorFor(cfg.DeepLKey)
	v.transSeq++
	seq := v.transSeq
	go func() {
		// Two DeepL requests of 10 s each, at most.
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		res, err := translate.WithFallback(ctx, tr, text, cfg.To, cfg.Fallback)
		if err != nil {
			applog.Write("goTranslate: %v", err)
		}
		payload := marshalResult(res, err)
		v.w.Dispatch(func() {
			if seq != v.transSeq {
				return // a newer request, or the page has changed
			}
			v.w.Eval("mdviewTranslateDone(" + payload + ")")
		})
	}()
	return ""
}
