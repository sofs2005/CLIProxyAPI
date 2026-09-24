package management

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestMinimalCodexRefreshPayloadSetsStoreFalse(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal(minimalCodexRefreshPayload(config.DefaultCodexFreeRefreshModel), &payload); err != nil {
		t.Fatalf("unmarshal minimal payload: %v", err)
	}
	if payload["model"] != config.DefaultCodexFreeRefreshModel {
		t.Fatalf("model = %v, want %s", payload["model"], config.DefaultCodexFreeRefreshModel)
	}
	store, ok := payload["store"].(bool)
	if !ok {
		t.Fatalf("store field type = %T, want bool", payload["store"])
	}
	if store {
		t.Fatalf("store = true, want false")
	}
}

func TestMinimalCodexRefreshPayloadUsesProvidedModel(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal(minimalCodexRefreshPayload("gpt-6-luna-mini"), &payload); err != nil {
		t.Fatalf("unmarshal minimal payload: %v", err)
	}
	if payload["model"] != "gpt-6-luna-mini" {
		t.Fatalf("model = %v, want gpt-6-luna-mini", payload["model"])
	}
}

func TestCodexFreeRefreshModelResolution(t *testing.T) {
	if got := (&config.CodexConfig{}).FreeRefreshModelOrDefault(); got != config.DefaultCodexFreeRefreshModel {
		t.Fatalf("empty config model = %q, want %q", got, config.DefaultCodexFreeRefreshModel)
	}
	if got := (&config.CodexConfig{FreeRefreshModel: "  "}).FreeRefreshModelOrDefault(); got != config.DefaultCodexFreeRefreshModel {
		t.Fatalf("blank config model = %q, want %q", got, config.DefaultCodexFreeRefreshModel)
	}
	if got := (&config.CodexConfig{FreeRefreshModel: " gpt-6-luna-mini "}).FreeRefreshModelOrDefault(); got != "gpt-6-luna-mini" {
		t.Fatalf("configured model = %q, want gpt-6-luna-mini", got)
	}
	var nilCfg *config.CodexConfig
	if got := nilCfg.FreeRefreshModelOrDefault(); got != config.DefaultCodexFreeRefreshModel {
		t.Fatalf("nil config model = %q, want %q", got, config.DefaultCodexFreeRefreshModel)
	}
}

func TestHandlerCodexFreeRefreshModelFallsBackWithoutConfig(t *testing.T) {
	if got := (*Handler)(nil).codexFreeRefreshModel(); got != config.DefaultCodexFreeRefreshModel {
		t.Fatalf("nil handler model = %q, want %q", got, config.DefaultCodexFreeRefreshModel)
	}
	if got := (&Handler{}).codexFreeRefreshModel(); got != config.DefaultCodexFreeRefreshModel {
		t.Fatalf("handler without config model = %q, want %q", got, config.DefaultCodexFreeRefreshModel)
	}
	configured := &Handler{cfg: &config.Config{Codex: config.CodexConfig{FreeRefreshModel: "gpt-6-luna-mini"}}}
	if got := configured.codexFreeRefreshModel(); got != "gpt-6-luna-mini" {
		t.Fatalf("configured handler model = %q, want gpt-6-luna-mini", got)
	}
}
