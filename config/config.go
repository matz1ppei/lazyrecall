package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AutoAdd struct {
	Enabled  bool   `json:"enabled"`
	Count    int    `json:"count"`
	Language string `json:"language"`  // ISO 639-1 code e.g. "es"
	LangName string `json:"lang_name"` // e.g. "Spanish"
}

type Notify struct {
	WebhookURL string `json:"webhook_url"`
}

type Config struct {
	AutoAdd AutoAdd `json:"auto_add"`
	Notify  Notify  `json:"notify"`
}

func DefaultConfig() Config {
	return Config{
		AutoAdd: AutoAdd{
			Enabled:  false,
			Count:    20,
			Language: "",
			LangName: "",
		},
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazyrecall", "config.json"), nil
}

func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return DefaultConfig(), nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	if cfg.AutoAdd.Count <= 0 {
		cfg.AutoAdd.Count = 20
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
