package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestCleanCodex401AuthFiles_DeletesOnlyCodex401(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "Bearer bad-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(probeServer.Close)

	originalProbeURL := codex401ProbeURL
	codex401ProbeURL = probeServer.URL
	t.Cleanup(func() { codex401ProbeURL = originalProbeURL })

	authDir := t.TempDir()
	badFile := filepath.Join(authDir, "codex-bad.json")
	goodFile := filepath.Join(authDir, "codex-good.json")
	otherFile := filepath.Join(authDir, "claude-bad.json")
	for path := range map[string]struct{}{badFile: {}, goodFile: {}, otherFile: {}} {
		if err := os.WriteFile(path, []byte(`{"type":"codex"}`), 0o600); err != nil {
			t.Fatalf("write auth file %s: %v", path, err)
		}
	}

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	for _, auth := range []*coreauth.Auth{
		{
			ID:       "codex-bad.json",
			FileName: "codex-bad.json",
			Provider: "codex",
			Attributes: map[string]string{
				"path": badFile,
			},
			Metadata: map[string]any{
				"type":         "codex",
				"access_token": "bad-token",
				"account_id":   "acct-bad",
			},
		},
		{
			ID:       "codex-good.json",
			FileName: "codex-good.json",
			Provider: "codex",
			Attributes: map[string]string{
				"path": goodFile,
			},
			Metadata: map[string]any{
				"type":         "codex",
				"access_token": "good-token",
				"account_id":   "acct-good",
			},
		},
		{
			ID:       "claude-bad.json",
			FileName: "claude-bad.json",
			Provider: "claude",
			Attributes: map[string]string{
				"path": otherFile,
			},
			Metadata: map[string]any{
				"type":         "claude",
				"access_token": "bad-token",
			},
		},
	} {
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("register auth %s: %v", auth.FileName, err)
		}
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/clean-codex-401", nil)

	h.CleanCodex401AuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Scanned    int `json:"scanned"`
		Matched401 int `json:"matched_401"`
		Deleted    int `json:"deleted"`
		Failed     int `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Scanned != 2 {
		t.Fatalf("expected scanned=2, got %d", payload.Scanned)
	}
	if payload.Matched401 != 1 {
		t.Fatalf("expected matched_401=1, got %d", payload.Matched401)
	}
	if payload.Deleted != 1 {
		t.Fatalf("expected deleted=1, got %d", payload.Deleted)
	}
	if payload.Failed != 0 {
		t.Fatalf("expected failed=0, got %d", payload.Failed)
	}

	if _, err := os.Stat(badFile); !os.IsNotExist(err) {
		t.Fatalf("expected bad codex file removed, stat err: %v", err)
	}
	if _, err := os.Stat(goodFile); err != nil {
		t.Fatalf("expected good codex file kept, stat err: %v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("expected non-codex file kept, stat err: %v", err)
	}
}

func TestCleanCodex401AuthFiles_ReportsDeleteFailure(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(probeServer.Close)

	originalProbeURL := codex401ProbeURL
	codex401ProbeURL = probeServer.URL
	t.Cleanup(func() { codex401ProbeURL = originalProbeURL })

	authDir := t.TempDir()
	missingFile := filepath.Join(authDir, "codex-missing.json")

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	auth := &coreauth.Auth{
		ID:       "codex-missing.json",
		FileName: "codex-missing.json",
		Provider: "codex",
		Attributes: map[string]string{
			"path": missingFile,
		},
		Metadata: map[string]any{
			"type":         "codex",
			"access_token": "bad-token",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/clean-codex-401", nil)

	h.CleanCodex401AuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Matched401 int                      `json:"matched_401"`
		Deleted    int                      `json:"deleted"`
		Failed     int                      `json:"failed"`
		Items      []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Matched401 != 1 {
		t.Fatalf("expected matched_401=1, got %d", payload.Matched401)
	}
	if payload.Deleted != 0 {
		t.Fatalf("expected deleted=0, got %d", payload.Deleted)
	}
	if payload.Failed != 1 {
		t.Fatalf("expected failed=1, got %d", payload.Failed)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(payload.Items))
	}
	if result, _ := payload.Items[0]["result"].(string); result != "delete_failed" {
		t.Fatalf("expected item result delete_failed, got %#v", payload.Items[0]["result"])
	}
}

func TestCleanCodex401AuthFiles_FallsBackToTokenStoreRecords(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(probeServer.Close)

	originalProbeURL := codex401ProbeURL
	codex401ProbeURL = probeServer.URL
	t.Cleanup(func() { codex401ProbeURL = originalProbeURL })

	authDir := t.TempDir()
	badFile := filepath.Join(authDir, "codex-store-only.json")
	if err := os.WriteFile(badFile, []byte(`{"type":"codex"}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	store := &memoryAuthStore{}
	store.items = map[string]*coreauth.Auth{
		"codex-store-only.json": {
			ID:       "codex-store-only.json",
			FileName: "codex-store-only.json",
			Provider: "codex",
			Attributes: map[string]string{
				"path": badFile,
			},
			Metadata: map[string]any{
				"type":         "codex",
				"access_token": "bad-token",
			},
		},
	}

	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/clean-codex-401", nil)

	h.CleanCodex401AuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Scanned int `json:"scanned"`
		Deleted int `json:"deleted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Scanned != 1 {
		t.Fatalf("expected scanned=1 from token store fallback, got %d", payload.Scanned)
	}
	if payload.Deleted != 1 {
		t.Fatalf("expected deleted=1 from token store fallback, got %d", payload.Deleted)
	}
	if _, err := os.Stat(badFile); !os.IsNotExist(err) {
		t.Fatalf("expected store-only file removed, stat err: %v", err)
	}
}

func TestCleanCodex401AuthFiles_HandlesWatcherStyleAuthWithoutFileName(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(probeServer.Close)

	originalProbeURL := codex401ProbeURL
	codex401ProbeURL = probeServer.URL
	t.Cleanup(func() { codex401ProbeURL = originalProbeURL })

	authDir := t.TempDir()
	badFile := filepath.Join(authDir, "codex-watcher-style.json")
	if err := os.WriteFile(badFile, []byte(`{"type":"codex"}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	auth := &coreauth.Auth{
		ID:       "codex-watcher-style.json",
		Provider: "codex",
		Attributes: map[string]string{
			"path": badFile,
		},
		Metadata: map[string]any{
			"type":         "codex",
			"access_token": "bad-token",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = store

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/clean-codex-401", nil)

	h.CleanCodex401AuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Scanned int `json:"scanned"`
		Deleted int `json:"deleted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Scanned != 1 {
		t.Fatalf("expected scanned=1 for watcher-style auth, got %d", payload.Scanned)
	}
	if payload.Deleted != 1 {
		t.Fatalf("expected deleted=1 for watcher-style auth, got %d", payload.Deleted)
	}
	if _, err := os.Stat(badFile); !os.IsNotExist(err) {
		t.Fatalf("expected watcher-style file removed, stat err: %v", err)
	}
}
