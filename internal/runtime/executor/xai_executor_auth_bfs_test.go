package executor

import (
	"encoding/base64"
	"testing"

	xaiauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/xai"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestApplyXAIBFSAttributeRefreshState(t *testing.T) {
	auth := &cliproxyauth.Auth{
		Provider: "xai",
		Attributes: map[string]string{
			cliproxyauth.AttributeAuthKind: cliproxyauth.AuthKindOAuth,
		},
		Metadata: map[string]any{
			"access_token": testRefreshXAIBFSToken(`{"bfs":1}`),
		},
	}
	applyXAIBFSAttribute(auth)
	if got := auth.Attributes[cliproxyauth.AttributeXAIBFS]; got != "true" {
		t.Fatalf("BFS marker after refresh = %q, want true", got)
	}

	auth.Metadata["access_token"] = testRefreshXAIBFSToken(`{"bfs":0}`)
	applyXAIBFSAttribute(auth)
	if _, ok := auth.Attributes[cliproxyauth.AttributeXAIBFS]; ok {
		t.Fatal("BFS marker was not cleared after non-BFS refresh")
	}
}

func TestApplyXAIBFSAttributeIgnoresNonOAuthAuth(t *testing.T) {
	auth := &cliproxyauth.Auth{
		Provider: "xai",
		Attributes: map[string]string{
			cliproxyauth.AttributeAuthKind: cliproxyauth.AuthKindAPIKey,
		},
		Metadata: map[string]any{
			"access_token": testRefreshXAIBFSToken(`{"bfs":1}`),
		},
	}
	applyXAIBFSAttribute(auth)
	if _, ok := auth.Attributes[cliproxyauth.AttributeXAIBFS]; ok {
		t.Fatal("BFS marker was set on API key auth")
	}
	if xaiauth.IsBFSAccessToken(auth.Metadata["access_token"].(string)) != true {
		t.Fatal("test token did not contain BFS marker")
	}
}

func testRefreshXAIBFSToken(claims string) string {
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
}
