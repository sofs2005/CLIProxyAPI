package synthesizer

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestSynthesizeAuthFileAppliesXAIBFSAttribute(t *testing.T) {
	bfsToken := testXAIBFSToken(`{"bfs":1}`)
	auths, errSynthesize := SynthesizeAuthFile(
		&SynthesisContext{Config: &config.Config{}, AuthDir: t.TempDir(), Now: time.Now()},
		"xai.json",
		[]byte(`{"type":"xai","auth_kind":"oauth","access_token":"`+bfsToken+`"}`),
	)
	if errSynthesize != nil {
		t.Fatalf("SynthesizeAuthFile() error = %v", errSynthesize)
	}
	if len(auths) != 1 {
		t.Fatalf("SynthesizeAuthFile() returned %d auths, want 1", len(auths))
	}
	if got := auths[0].Attributes[coreauth.AttributeXAIBFS]; got != "true" {
		t.Fatalf("xAI BFS attribute = %q, want true", got)
	}
}

func TestApplyXAIBFSAttributeClearsStaleMarker(t *testing.T) {
	auth := &coreauth.Auth{
		Provider: "xai",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: coreauth.AuthKindOAuth,
			coreauth.AttributeXAIBFS:   "true",
		},
	}
	applyXAIBFSAttribute(auth, "xai", map[string]any{
		"auth_kind":    "oauth",
		"access_token": testXAIBFSToken(`{"bfs":0}`),
	})
	if _, ok := auth.Attributes[coreauth.AttributeXAIBFS]; ok {
		t.Fatal("stale xAI BFS attribute was not cleared")
	}
}

func TestApplyXAIBFSAttributeIgnoresAPIKeyAuth(t *testing.T) {
	auth := &coreauth.Auth{
		Provider: "xai",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
		},
	}
	applyXAIBFSAttribute(auth, "xai", map[string]any{
		"auth_kind":    "oauth",
		"access_token": testXAIBFSToken(`{"bfs":1}`),
	})
	if _, ok := auth.Attributes[coreauth.AttributeXAIBFS]; ok {
		t.Fatal("xAI BFS attribute was set on API key auth")
	}
}

func TestSynthesizePluginAuthAppliesXAIBFSAttribute(t *testing.T) {
	parser := testXAIBFSPluginParser{
		auth: &coreauth.Auth{
			Provider: "xai",
			Metadata: map[string]any{
				"auth_kind":    "oauth",
				"access_token": testXAIBFSToken(`{"bfs":1}`),
			},
			Attributes: map[string]string{
				coreauth.AttributeAuthKind: coreauth.AuthKindOAuth,
			},
		},
	}
	auths, errSynthesize := SynthesizeAuthFile(
		&SynthesisContext{
			Config:           &config.Config{},
			AuthDir:          t.TempDir(),
			Now:              time.Now(),
			PluginAuthParser: parser,
		},
		"xai.json",
		[]byte(`{"type":"xai","auth_kind":"oauth","access_token":"ignored"}`),
	)
	if errSynthesize != nil {
		t.Fatalf("SynthesizeAuthFile() error = %v", errSynthesize)
	}
	if len(auths) != 1 {
		t.Fatalf("SynthesizeAuthFile() returned %d auths, want 1", len(auths))
	}
	if got := auths[0].Attributes[coreauth.AttributeXAIBFS]; got != "true" {
		t.Fatalf("plugin xAI BFS attribute = %q, want true", got)
	}
}

type testXAIBFSPluginParser struct {
	auth *coreauth.Auth
}

func (p testXAIBFSPluginParser) ParseAuth(context.Context, pluginapi.AuthParseRequest) (*coreauth.Auth, bool, error) {
	return p.auth.Clone(), true, nil
}

func testXAIBFSToken(claims string) string {
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
}
