package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Theme    string `json:"theme"`    // theme id, e.g. "light", "dark", "dracula"
	Font     string `json:"font"`     // font id, e.g. "sans", "serif", "reading", "mono"
	FontSize int    `json:"fontSize"` // body font size in px
}

// Default returns the built-in default preferences.
func Default() Config {
	return Config{Theme: "light", Font: "sans", FontSize: 18}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mdview", "config.json"), nil
}

func Load() Config {
	cfg := Default()
	path, err := configPath()
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

func Save(cfg Config) {
	path, err := configPath()
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.Marshal(cfg)
	os.WriteFile(path, data, 0644)
}
