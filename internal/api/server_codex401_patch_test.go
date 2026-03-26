package api

import (
	"strings"
	"testing"
)

func TestInjectAuthFilesWarningFilterPatch_ContainsCleanerAction(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectAuthFilesWarningFilterPatch(input)
	result := string(out)

	if !strings.Contains(result, "cpa-auth-clean-401-button") {
		t.Fatal("expected clean-401 button marker in auth-files patch output")
	}
	if !strings.Contains(result, "/v0/management/auth-files/clean-codex-401") {
		t.Fatal("expected clean-codex-401 endpoint in auth-files patch output")
	}
}

func TestInjectAuthFilesWarningFilterPatch_ContainsCodexQuotaSyncHooks(t *testing.T) {
	input := []byte("<html><body><div>content</div></body></html>")
	out := injectAuthFilesWarningFilterPatch(input)
	result := string(out)

	for _, needle := range []string{
		"cpa-auth-clean-401-button",
		"/v0/management/auth-files/clean-codex-401",
		"captureHeaders",
	} {
		if !strings.Contains(result, needle) {
			t.Fatalf("expected auth warning filter patch to include %q", needle)
		}
	}
	for _, needle := range []string{
		"/v0/management/auth-files/codex-quota-sync",
		"backend-api/wham/usage",
		"codexQuotaSync",
	} {
		if strings.Contains(result, needle) {
			t.Fatalf("expected auth warning filter patch to no longer include %q", needle)
		}
	}
}
