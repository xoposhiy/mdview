package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(DirEnv, dir)
	return filepath.Join(dir, "config.json")
}

func TestLoadWithoutTranslateGivesDefaults(t *testing.T) {
	path := useTempDir(t)
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	if cfg.Theme != "dark" {
		t.Errorf("Theme = %q, want dark", cfg.Theme)
	}
	if cfg.Translate.To != "RU" || cfg.Translate.Fallback != "EN" {
		t.Errorf("Translate = %+v, want To=RU Fallback=EN", cfg.Translate)
	}
	if cfg.Translate.DeepLKey != "" {
		t.Errorf("DeepLKey = %q, want empty", cfg.Translate.DeepLKey)
	}
}

func TestLoadReadsTranslate(t *testing.T) {
	path := useTempDir(t)
	data := `{"translate":{"deeplKey":"k:fx","to":"DE"}}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	tr := Load().Translate
	if tr.DeepLKey != "k:fx" || tr.To != "DE" || tr.Fallback != "EN" {
		t.Errorf("Translate = %+v", tr)
	}
}

func readMap(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("file is not a JSON object: %v\n%s", err, data)
	}
	return m
}

func TestSaveKeepsTranslateFromDisk(t *testing.T) {
	path := useTempDir(t)
	if err := os.WriteFile(path, []byte(`{"theme":"light","translate":{"deeplKey":"secret:fx"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := Default() // in-memory copy without the key
	cfg.Theme = "dark"
	Save(cfg)

	got := Load()
	if got.Theme != "dark" {
		t.Errorf("Theme = %q, want dark", got.Theme)
	}
	if got.Translate.DeepLKey != "secret:fx" {
		t.Errorf("DeepLKey = %q, want secret:fx", got.Translate.DeepLKey)
	}
}

func TestSaveKeepsUnknownKey(t *testing.T) {
	path := useTempDir(t)
	if err := os.WriteFile(path, []byte(`{"custom":{"a":1}}`), 0644); err != nil {
		t.Fatal(err)
	}
	Save(Default())
	m := readMap(t, path)
	var custom bytes.Buffer
	json.Compact(&custom, m["custom"])
	if custom.String() != `{"a":1}` {
		t.Errorf("custom = %s, want {\"a\":1}", m["custom"])
	}
	if string(m["theme"]) != `"light"` {
		t.Errorf("theme = %s, want \"light\"", m["theme"])
	}
}

func TestSaveCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	t.Setenv(DirEnv, sub)
	cfg := Default()
	cfg.FontSize = 21
	Save(cfg)

	m := readMap(t, filepath.Join(sub, "config.json"))
	for _, k := range []string{"theme", "font", "fontSize", "imageBg"} {
		if _, ok := m[k]; !ok {
			t.Errorf("key %q missing", k)
		}
	}
	if string(m["fontSize"]) != "21" {
		t.Errorf("fontSize = %s, want 21", m["fontSize"])
	}
}

func TestPathUsesDirEnv(t *testing.T) {
	path := useTempDir(t)
	got, err := Path()
	if err != nil || got != path {
		t.Errorf("Path() = %q, %v; want %q", got, err, path)
	}
}

func TestSaveLeavesInvalidFileAlone(t *testing.T) {
	path := useTempDir(t)
	broken := `{"translate":{"deeplKey":"k:fx",}` // a hand edit in progress
	if err := os.WriteFile(path, []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	Save(Default())
	data, _ := os.ReadFile(path)
	if string(data) != broken {
		t.Errorf("file changed to %s", data)
	}
}
