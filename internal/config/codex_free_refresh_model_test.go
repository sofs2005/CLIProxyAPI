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

// The canonical v8 location is upstream.codex; oauth.providers.codex is the
// historical path and must migrate to it.
func TestParseConfigBytes_CodexFreeRefreshModelLocations(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{
			name: "canonical upstream path",
			raw:  "config-version: 8\nupstream:\n  codex:\n    free-refresh-model: \"  gpt-9-test  \"\n",
		},
		{
			name: "historical oauth path",
			raw:  "config-version: 8\noauth:\n  providers:\n    codex:\n      free-refresh-model: \"  gpt-9-test  \"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, errParse := ParseConfigBytes([]byte(tc.raw))
			if errParse != nil {
				t.Fatalf("ParseConfigBytes() error = %v", errParse)
			}
			if got := cfg.Codex.FreeRefreshModelOrDefault(); got != "gpt-9-test" {
				t.Fatalf("FreeRefreshModelOrDefault() = %q, want %q", got, "gpt-9-test")
			}
		})
	}
}
