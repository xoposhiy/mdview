package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Theme string `json:"theme"` // "light" or "dark"
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mdview", "config.json"), nil
}

func Load() Config {
	cfg := Config{Theme: "light"}
	path, err := configPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
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
