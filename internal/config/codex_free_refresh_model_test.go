package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOptional_CodexFreeRefreshModel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
codex:
  free-refresh-model: "  gpt-6-luna-mini  "
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}
	if got := cfg.Codex.FreeRefreshModelOrDefault(); got != "gpt-6-luna-mini" {
		t.Fatalf("FreeRefreshModelOrDefault() = %q, want %q", got, "gpt-6-luna-mini")
	}
}

func TestLoadConfigOptional_CodexFreeRefreshModelDefault(t *testing.T) {
	defaultCfg, errDefault := ParseConfigBytes([]byte(`{}`))
	if errDefault != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errDefault)
	}
	if got := defaultCfg.Codex.FreeRefreshModelOrDefault(); got != DefaultCodexFreeRefreshModel {
		t.Fatalf("default FreeRefreshModelOrDefault() = %q, want %q", got, DefaultCodexFreeRefreshModel)
	}
}
