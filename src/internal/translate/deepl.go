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
	deeplFree = "https://api-free.deepl.com/v2/translate"
	deeplPro  = "https://api.deepl.com/v2/translate"
)

// DeepL translates through the DeepL API.
type DeepL struct {
	key      string
	endpoint string
	client   *http.Client
}

// NewDeepL returns a client for key. Free keys end with ":fx" and use a
// different host, so the host follows the key.
func NewDeepL(key string) *DeepL {
	endpoint := deeplPro
	if strings.HasSuffix(key, ":fx") {
		endpoint = deeplFree
	}
	// The zero http.Client has no timeout; the spinner would then spin
	// forever when the network hangs.
	return &DeepL{key: key, endpoint: endpoint, client: &http.Client{Timeout: 10 * time.Second}}
}

type deeplRequest struct {
	Text       []string `json:"text"`
	TargetLang string   `json:"target_lang"`
}

type deeplResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
	Message string `json:"message"`
}

func (d *DeepL) Translate(ctx context.Context, text, target string) (Result, error) {
	body, err := json.Marshal(deeplRequest{Text: []string{text}, TargetLang: target})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+d.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden:
		return Result{}, ErrAuth
	case http.StatusRequestEntityTooLarge:
		return Result{}, ErrTooLong
	case http.StatusTooManyRequests, 529:
		return Result{}, ErrRateLimit
	case 456:
		return Result{}, ErrQuota
	case http.StatusInternalServerError, http.StatusGatewayTimeout:
		return Result{}, ErrTransient
	default:
		var r deeplResponse
		json.Unmarshal(data, &r)
		return Result{}, &StatusError{Code: resp.StatusCode, Msg: r.Message}
	}

	var r deeplResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return Result{}, fmt.Errorf("bad response: %w", err)
	}
	if len(r.Translations) == 0 {
		return Result{}, fmt.Errorf("bad response: no translations")
	}
	tr := r.Translations[0]
	return Result{Text: tr.Text, Source: tr.DetectedSourceLanguage, Target: target}, nil
}
