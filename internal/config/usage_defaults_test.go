package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaultsUsageStatisticsEnabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.UsageStatisticsEnabled {
		t.Fatalf("expected UsageStatisticsEnabled default to be true")
	}
}

func TestLoadConfigRespectsUsageStatisticsExplicitFalse(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := "port: 8317\nusage-statistics-enabled: false\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.UsageStatisticsEnabled {
		t.Fatalf("expected UsageStatisticsEnabled to remain false when configured")
	}
}
