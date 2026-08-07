package auth

import (
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func TestResolveXAIBFSModelAlias(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{
		Provider: "xai",
		Attributes: map[string]string{
			AttributeAuthKind: AuthKindOAuth,
			AttributeXAIBFS:   "true",
		},
	}

	result := manager.resolveOAuthModelAliasWithResult(auth, "grok-model-ks(high)")
	if result.UpstreamModel != "grok-model(high)" || !result.ForceMapping || result.OriginalAlias != "grok-model-ks(high)" {
		t.Fatalf("BFS alias result = %#v", result)
	}
}

func TestResolveXAIBFSModelAliasUsesOAuthAlias(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
		"xai": {{Name: "grok-4", Alias: "fast", ForceMapping: false}},
	})
	auth := &Auth{
		Provider: "xai",
		Attributes: map[string]string{
			AttributeAuthKind: AuthKindOAuth,
			AttributeXAIBFS:   "true",
		},
	}

	result := manager.resolveOAuthModelAliasWithResult(auth, "fast-ks(8192)")
	if result.UpstreamModel != "grok-4(8192)" || !result.ForceMapping || result.OriginalAlias != "fast-ks(8192)" {
		t.Fatalf("BFS OAuth alias result = %#v", result)
	}
}

func TestResolveXAIBFSModelAliasDoesNotAffectNormalAuth(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{
		Provider: "xai",
		Attributes: map[string]string{
			AttributeAuthKind: AuthKindOAuth,
		},
	}
	result := manager.resolveOAuthModelAliasWithResult(auth, "grok-model-ks")
	if result.UpstreamModel != "" || result.ForceMapping || result.OriginalAlias != "" {
		t.Fatalf("normal auth unexpectedly resolved BFS alias: %#v", result)
	}
}

func TestXAIBFSAuthSupportsOnlySuffixedRouteModel(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "xai-bfs-selection-test",
		Provider: "xai",
		Attributes: map[string]string{
			AttributeAuthKind: AuthKindOAuth,
			AttributeXAIBFS:   "true",
		},
	}
	registryRef := registry.GetGlobalRegistry()
	registryRef.RegisterClient(auth.ID, "xai", []*registry.ModelInfo{{ID: "grok-model-ks"}})
	defer registryRef.UnregisterClient(auth.ID)

	if manager.authSupportsRouteModel(registryRef, auth, "grok-model") {
		t.Fatal("BFS auth unexpectedly supports raw model")
	}
	if !manager.authSupportsRouteModel(registryRef, auth, "grok-model-ks") {
		t.Fatal("BFS auth does not support suffixed model")
	}
}

func TestXAIBFSForceMappingRewritesNonStreamAndStreamModels(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{
		Provider: "xai",
		Attributes: map[string]string{
			AttributeAuthKind: AuthKindOAuth,
			AttributeXAIBFS:   "true",
		},
	}
	alias := manager.resolveOAuthModelAliasWithResult(auth, "grok-model-ks")
	response := &cliproxyexecutor.Response{Payload: []byte(`{"model":"grok-model","choices":[]}`)}
	rewriteForceMappedResponse(response, alias)
	if string(response.Payload) != `{"model":"grok-model-ks","choices":[]}` {
		t.Fatalf("non-stream response = %s", response.Payload)
	}

	rewriter := NewStreamRewriter(StreamRewriteOptions{RewriteModel: alias.OriginalAlias})
	chunk := rewriter.RewriteChunk([]byte("data: {\"model\":\"grok-model\"}\n\n"))
	if string(chunk) != "data: {\"model\":\"grok-model-ks\"}\n\n" {
		t.Fatalf("stream response = %q", chunk)
	}
}
