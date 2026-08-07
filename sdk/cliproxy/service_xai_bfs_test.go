package cliproxy

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestRegisterResolvedModelsForAuthAppendsXAIBFSSuffix(t *testing.T) {
	service := &Service{}
	auth := &coreauth.Auth{
		ID:       "xai-bfs-registration-test",
		Provider: "xai",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: coreauth.AuthKindOAuth,
			coreauth.AttributeXAIBFS:   "true",
		},
	}
	defer GlobalModelRegistry().UnregisterClient(auth.ID)

	service.registerResolvedModelsForAuth(auth, "xai", []*ModelInfo{
		{ID: "prefix/grok-model", Name: "models/grok-model", DisplayName: "prefix/grok-model"},
		{ID: "already-ks", Name: "already-ks", DisplayName: "Human readable"},
	})

	models := registry.GetGlobalRegistry().GetModelsForClient(auth.ID)
	got := make(map[string]ModelInfo, len(models))
	for _, model := range models {
		if model != nil {
			got[model.ID] = *model
		}
	}
	if _, ok := got["prefix/grok-model-ks"]; !ok {
		t.Fatalf("registered models = %v, missing BFS model", modelIDs(models))
	}
	if model := got["prefix/grok-model-ks"]; model.Name != "models/grok-model-ks" || model.DisplayName != "prefix/grok-model-ks" {
		t.Fatalf("BFS model metadata = %#v", model)
	}
	if _, ok := got["already-ks"]; !ok {
		t.Fatalf("registered models = %v, missing idempotent model", modelIDs(models))
	}
	if _, ok := got["prefix/grok-model"]; ok {
		t.Fatalf("raw model unexpectedly registered: %v", modelIDs(models))
	}
}

func TestRegisterResolvedModelsForAuthLeavesNormalXAIUnchanged(t *testing.T) {
	service := &Service{}
	auth := &coreauth.Auth{
		ID:       "xai-normal-registration-test",
		Provider: "xai",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: coreauth.AuthKindOAuth,
		},
	}
	defer GlobalModelRegistry().UnregisterClient(auth.ID)

	service.registerResolvedModelsForAuth(auth, "xai", []*ModelInfo{{ID: "grok-model"}})
	models := registry.GetGlobalRegistry().GetModelsForClient(auth.ID)
	if len(models) != 1 || models[0] == nil || models[0].ID != "grok-model" {
		t.Fatalf("registered normal models = %v, want grok-model", modelIDs(models))
	}
}

func modelIDs(models []*ModelInfo) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if model != nil {
			ids = append(ids, model.ID)
		}
	}
	return ids
}
