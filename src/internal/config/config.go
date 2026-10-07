package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Config struct {
	Theme    string `json:"theme"`    // theme id, e.g. "light", "dark", "dracula"
	Font     string `json:"font"`     // font id, e.g. "sans", "serif", "reading", "mono"
	FontSize int    `json:"fontSize"` // body font size in px
	ImageBg  string `json:"imageBg"`  // letterbox background behind images: "black" or "white"

	// Translate is edited by hand in the file. Save never writes it.
	Translate Translate `json:"translate"`
}

// Translate holds the settings for translating the selection.
type Translate struct {
	DeepLKey string `json:"deeplKey"`
	To       string `json:"to"`       // target language, e.g. "RU"
	Fallback string `json:"fallback"` // target when the text is already in To
}

// Default returns the built-in default preferences.
func Default() Config {
	return Config{
		Theme: "light", Font: "sans", FontSize: 18, ImageBg: "black",
		Translate: Translate{To: "RU", Fallback: "EN"},
	}
}

// DirEnv names an environment variable that moves the preferences file
// elsewhere. An automated check sets it, so that driving the real application
// does not overwrite the person's own theme, font and background choices.
const DirEnv = "MDVIEW_CONFIG_DIR"

// Path returns the full path of the preferences file.
func Path() (string, error) {
	if dir := os.Getenv(DirEnv); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mdview", "config.json"), nil
}

func Load() Config {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	// Unmarshal over defaults so fields absent from older config files keep
	// their default values.
	json.Unmarshal(data, &cfg)
	return cfg
}

// Save writes the appearance fields of cfg into the file. Other keys in the
// file, such as "translate" or keys the user added by hand, stay as they are
// on disk: the user may edit the file while mdview runs, and the in-memory
// copy is then out of date.
func Save(cfg Config) {
	path, err := Path()
	if err != nil {
		return
	}
	m := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if json.Unmarshal(data, &m) != nil {
			// The user is in the middle of an edit. Overwriting the file
			// would lose it; losing one theme change is better.
			return
		}
		if m == nil { // the file holds "null"
			m = map[string]json.RawMessage{}
		}
	case !errors.Is(err, fs.ErrNotExist):
		return
	}
	set := func(key string, v any) {
		b, _ := json.Marshal(v)
		m[key] = b
	}
	set("theme", cfg.Theme)
	set("font", cfg.Font)
	set("fontSize", cfg.FontSize)
	set("imageBg", cfg.ImageBg)

	os.MkdirAll(filepath.Dir(path), 0755)
	out, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(path, out, 0644)
}
