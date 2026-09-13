package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "config.toml")

	tomlContent := `
arl = "test_arl_from_file"
quality = "flac"
dir = "/custom/downloads"
workers = 4
skip = true
sync = true
lyrics = true
lrc = true
`
	if err := os.WriteFile(confPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, path, err := loadConfig(confPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if path != confPath {
		t.Errorf("expected path %s, got %s", confPath, path)
	}
	if cfg.ARL != "test_arl_from_file" {
		t.Errorf("expected ARL test_arl_from_file, got %s", cfg.ARL)
	}
	if cfg.Quality != "flac" {
		t.Errorf("expected Quality flac, got %s", cfg.Quality)
	}
	if cfg.Dir != "/custom/downloads" {
		t.Errorf("expected Dir /custom/downloads, got %s", cfg.Dir)
	}
	if cfg.Workers != 4 {
		t.Errorf("expected Workers 4, got %d", cfg.Workers)
	}
	if !cfg.Skip || !cfg.Sync || !cfg.Lyrics || !cfg.LRC {
		t.Errorf("expected booleans to be true, got %+v", cfg)
	}
}

func TestLoadConfigNonExistent(t *testing.T) {
	cfg, _, err := loadConfig("/path/to/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("expected nil error on nonexistent file, got: %v", err)
	}
	if cfg.ARL != "" || cfg.Workers != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestConfigHelperFunctions(t *testing.T) {
	if firstNonEmpty("", "", "first", "second") != "first" {
		t.Error("firstNonEmpty failed")
	}
	if stringDefault("", "fallback") != "fallback" {
		t.Error("stringDefault fallback failed")
	}
	if stringDefault("val", "fallback") != "val" {
		t.Error("stringDefault value failed")
	}
	if intDefault(0, 8) != 8 {
		t.Error("intDefault fallback failed")
	}
	if intDefault(4, 8) != 4 {
		t.Error("intDefault value failed")
	}
}
