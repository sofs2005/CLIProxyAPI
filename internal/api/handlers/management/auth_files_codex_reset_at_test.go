package management

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// codexQuotaResetMetadata builds the codex_quota metadata shape read by
// coreauth.CodexPrimaryResetAt.
func codexQuotaResetMetadata(resetAt any) map[string]any {
	return map[string]any{
		"codex_quota": map[string]any{
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 100,
					"reset_at":     resetAt,
				},
			},
		},
	}
}

// registerCodexAuthWithReset registers a Codex credential backed by a real file so
// the entry builder lists it, with the given codex_quota reset value.
func registerCodexAuthWithReset(t *testing.T, manager *coreauth.Manager, authDir, id string, resetAt any) {
	t.Helper()

	fileName := id + ".json"
	filePath := filepath.Join(authDir, fileName)
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"codex"}`), 0o600); errWrite != nil {
		t.Fatalf("write auth file %s: %v", fileName, errWrite)
	}
	auth := &coreauth.Auth{
		ID:       id,
		Index:    "idx-" + id,
		FileName: fileName,
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"path": filePath,
		},
	}
	if resetAt != nil {
		auth.Metadata = codexQuotaResetMetadata(resetAt)
	}
	if _, errRegister := manager.Register(context.Background(), auth); errRegister != nil {
		t.Fatalf("register %s: %v", id, errRegister)
	}
}

func TestListAuthFilesExposesCodexResetAtForFutureReset(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	authDir := t.TempDir()
	manager := coreauth.NewManager(nil, nil, nil)
	resetAt := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	registerCodexAuthWithReset(t, manager, authDir, "codex-future", resetAt.Format(time.RFC3339))
	handler := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)

	payload := requestAuthFilesPage(t, handler, "/v0/management/auth-files")
	if len(payload.Files) != 1 {
		t.Fatalf("files len = %d, want 1", len(payload.Files))
	}
	got, ok := payload.Files[0]["codex_reset_at"].(string)
	if !ok {
		t.Fatalf("codex_reset_at = %#v, want a string", payload.Files[0]["codex_reset_at"])
	}
	parsed, errParse := time.Parse(time.RFC3339, got)
	if errParse != nil {
		t.Fatalf("codex_reset_at = %q is not RFC3339: %v", got, errParse)
	}
	if !parsed.Equal(resetAt) {
		t.Fatalf("codex_reset_at = %s, want %s", parsed, resetAt)
	}
}

func TestListAuthFilesOmitsCodexResetAtForExpiredOrInvalidReset(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	cases := map[string]any{
		"elapsed":    time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
		"unparsable": "not-a-time",
		"missing":    nil,
	}
	for name, resetAt := range cases {
		t.Run(name, func(t *testing.T) {
			authDir := t.TempDir()
			manager := coreauth.NewManager(nil, nil, nil)
			registerCodexAuthWithReset(t, manager, authDir, "codex-"+name, resetAt)
			handler := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)

			payload := requestAuthFilesPage(t, handler, "/v0/management/auth-files")
			if len(payload.Files) != 1 {
				t.Fatalf("files len = %d, want 1", len(payload.Files))
			}
			if _, present := payload.Files[0]["codex_reset_at"]; present {
				t.Fatalf("codex_reset_at = %#v, want the field to be omitted", payload.Files[0]["codex_reset_at"])
			}
		})
	}
}
