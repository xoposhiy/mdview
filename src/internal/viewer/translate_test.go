package viewer

import (
	"encoding/json"
	"strings"
	"testing"

	"mdview/internal/config"
	"mdview/internal/translate"
)

func TestMarshalResultOK(t *testing.T) {
	got := marshalResult(translate.Result{Text: "привет ", Source: "EN", Target: "RU"}, nil)
	if strings.ContainsRune(got, ' ') {
		t.Errorf("U+2028 is not escaped: %s", got)
	}
	var p translatePayload
	if err := json.Unmarshal([]byte(got), &p); err != nil {
		t.Fatal(err)
	}
	want := translatePayload{OK: true, Text: "привет ", Source: "EN", Target: "RU"}
	if p != want {
		t.Errorf("payload = %+v, want %+v", p, want)
	}
}

func TestMarshalResultError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.DirEnv, dir)
	var p translatePayload
	json.Unmarshal([]byte(marshalResult(translate.Result{}, translate.ErrNoKey)), &p)
	if p.OK || p.Retry || !strings.Contains(p.Msg, dir) {
		t.Errorf("no key: %+v", p)
	}
	json.Unmarshal([]byte(marshalResult(translate.Result{}, translate.ErrRateLimit)), &p)
	if p.OK || !p.Retry || p.Msg == "" {
		t.Errorf("rate limit: %+v", p)
	}
}
