package main

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all configurable options for the CLI.
// Values are merged with CLI flags at startup (flags take precedence).
type Config struct {
	ARL     string `toml:"arl"`
	Dir     string `toml:"dir"`
	Quality string `toml:"quality"`
	Workers int    `toml:"workers"`
	Skip    bool   `toml:"skip"`
	Sync    bool   `toml:"sync"`
	Lyrics  bool   `toml:"lyrics"`
	LRC     bool   `toml:"lrc"`
}

// defaultConfigPath returns ~/.config/music-downloader/config.toml.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "music-downloader", "config.toml")
}

// loadConfig loads a TOML config from path (or the default location if path is "").
// Returns a zero Config (and no error) if the file does not exist.
func loadConfig(path string) (Config, string, error) {
	if path == "" {
		path = defaultConfigPath()
	}
	var cfg Config
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, path, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}
