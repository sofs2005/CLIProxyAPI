package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gin "github.com/gin-gonic/gin"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	internallogging "github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/redisqueue"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithOptions(t)
}

func newTestServerWithOptions(t *testing.T, opts ...ServerOption) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port:                   0,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	configPath := filepath.Join(tmpDir, "config.yaml")
	return NewServer(cfg, authManager, accessManager, configPath, opts...)
}

func TestHealthz(t *testing.T) {
	server := newTestServer(t)

	t.Run("GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var resp struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v; body=%s", err, rr.Body.String())
		}
		if resp.Status != "ok" {
			t.Fatalf("unexpected response status: got %q want %q", resp.Status, "ok")
		}
	})

	t.Run("HEAD", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("expected empty body for HEAD request, got %q", rr.Body.String())
		}
	})
}

func TestManagementResponseExposesPluginSupportHeaderForCORS(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")

	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/config", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	if got := rr.Header().Get("X-CPA-SUPPORT-PLUGIN"); got != pluginhost.SupportPluginHeaderValue() {
		t.Fatalf("X-CPA-SUPPORT-PLUGIN = %q, want %q", got, pluginhost.SupportPluginHeaderValue())
	}

	exposedHeaders := make(map[string]struct{})
	for _, headerName := range strings.Split(rr.Header().Get("Access-Control-Expose-Headers"), ",") {
		headerName = strings.ToLower(strings.TrimSpace(headerName))
		if headerName != "" {
			exposedHeaders[headerName] = struct{}{}
		}
	}
	for _, headerName := range corsExposedResponseHeaders {
		if _, ok := exposedHeaders[strings.ToLower(headerName)]; !ok {
			t.Fatalf("Access-Control-Expose-Headers missing %s: %q", headerName, rr.Header().Get("Access-Control-Expose-Headers"))
		}
	}
}

func TestNewServerWithPluginHostInjectsHandlerInterceptors(t *testing.T) {
	host := pluginhost.New()
	server := newTestServerWithOptions(t, WithPluginHost(host))

	if server.handlers == nil {
		t.Fatal("server handlers = nil")
	}
	got, ok := server.handlers.PluginHost.(*pluginhost.Host)
	if !ok || got != host {
		t.Fatalf("handler plugin host = %#v, want configured host", server.handlers.PluginHost)
	}
}

func TestNewServerWithoutPluginHostLeavesHandlerInterceptorsDisabled(t *testing.T) {
	server := newTestServer(t)

	if server.handlers == nil {
		t.Fatal("server handlers = nil")
	}
	if server.handlers.PluginHost != nil {
		t.Fatalf("handler plugin host = %#v, want nil", server.handlers.PluginHost)
	}
}

func TestManagementUsageRequiresManagementAuthAndPopsArray(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")

	prevQueueEnabled := redisqueue.Enabled()
	redisqueue.SetEnabled(false)
	t.Cleanup(func() {
		redisqueue.SetEnabled(false)
		redisqueue.SetEnabled(prevQueueEnabled)
	})

	server := newTestServer(t)

	redisqueue.Enqueue([]byte(`{"id":1}`))
	redisqueue.Enqueue([]byte(`{"id":2}`))

	missingKeyReq := httptest.NewRequest(http.MethodGet, "/v0/management/usage-queue?count=2", nil)
	missingKeyRR := httptest.NewRecorder()
	server.engine.ServeHTTP(missingKeyRR, missingKeyReq)
	if missingKeyRR.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d, want %d body=%s", missingKeyRR.Code, http.StatusUnauthorized, missingKeyRR.Body.String())
	}

	legacyReq := httptest.NewRequest(http.MethodGet, "/v0/management/usage?count=2", nil)
	legacyReq.Header.Set("Authorization", "Bearer test-management-key")
	legacyRR := httptest.NewRecorder()
	server.engine.ServeHTTP(legacyRR, legacyReq)
	if legacyRR.Code != http.StatusNotFound {
		t.Fatalf("legacy usage status = %d, want %d body=%s", legacyRR.Code, http.StatusNotFound, legacyRR.Body.String())
	}

	authReq := httptest.NewRequest(http.MethodGet, "/v0/management/usage-queue?count=2", nil)
	authReq.Header.Set("Authorization", "Bearer test-management-key")
	authRR := httptest.NewRecorder()
	server.engine.ServeHTTP(authRR, authReq)
	if authRR.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d body=%s", authRR.Code, http.StatusOK, authRR.Body.String())
	}

	var payload []json.RawMessage
	if errUnmarshal := json.Unmarshal(authRR.Body.Bytes(), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal response: %v body=%s", errUnmarshal, authRR.Body.String())
	}
	if len(payload) != 2 {
		t.Fatalf("response records = %d, want 2", len(payload))
	}
	for i, raw := range payload {
		var record struct {
			ID int `json:"id"`
		}
		if errUnmarshal := json.Unmarshal(raw, &record); errUnmarshal != nil {
			t.Fatalf("unmarshal record %d: %v", i, errUnmarshal)
		}
		if record.ID != i+1 {
			t.Fatalf("record %d id = %d, want %d", i, record.ID, i+1)
		}
	}

	if remaining := redisqueue.PopOldest(1); len(remaining) != 0 {
		t.Fatalf("remaining queue = %q, want empty", remaining)
	}
}

func TestManagementControlPanelMissingAssetReturnsBootstrapWhileSyncSlow(t *testing.T) {
	staticDir := t.TempDir()
	t.Setenv("MANAGEMENT_STATIC_PATH", staticDir)

	releaseBlock := make(chan struct{})
	releaseRequested := make(chan struct{})
	var closeReleaseRequested sync.Once
	var connectCount atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect && strings.Contains(r.Host, "api.github.com") {
			connectCount.Add(1)
			closeReleaseRequested.Do(func() { close(releaseRequested) })
			<-releaseBlock
			http.Error(w, "release endpoint unavailable", http.StatusBadGateway)
			return
		}
		http.Error(w, "unexpected proxy request", http.StatusBadGateway)
	}))
	t.Cleanup(func() {
		select {
		case <-releaseBlock:
		default:
			close(releaseBlock)
		}
		proxyServer.Close()
	})

	server := newTestServer(t)
	server.cfg.ProxyURL = proxyServer.URL

	req := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	start := time.Now()
	go func() {
		server.engine.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Fatalf("/management.html took %s with missing asset and slow release endpoint", elapsed)
		}
	case <-time.After(200 * time.Millisecond):
		close(releaseBlock)
		<-done
		t.Fatalf("/management.html blocked while synchronously syncing missing management asset")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if cacheControl := rr.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Management panel is loading") || !strings.Contains(body, "location.reload") {
		t.Fatalf("expected loading bootstrap HTML, got %s", body)
	}

	select {
	case <-releaseRequested:
	case <-time.After(time.Second):
		t.Fatal("background management asset sync was not started")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	secondRR := httptest.NewRecorder()
	secondDone := make(chan struct{})
	go func() {
		server.engine.ServeHTTP(secondRR, secondReq)
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(200 * time.Millisecond):
		close(releaseBlock)
		<-secondDone
		t.Fatal("second /management.html request blocked while sync was already running")
	}
	if got := connectCount.Load(); got != 1 {
		t.Fatalf("release endpoint CONNECT count = %d, want 1", got)
	}
	if secondRR.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d body=%s", secondRR.Code, http.StatusOK, secondRR.Body.String())
	}

	close(releaseBlock)
}

func TestServeManagementControlPanel_ETagAndConditionalRequest(t *testing.T) {
	server := newTestServer(t)
	server.cfg.RemoteManagement.SecretKey = "test"

	staticDir := filepath.Join(filepath.Dir(server.configFilePath), "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}
	filePath := filepath.Join(staticDir, "management.html")
	if err := os.WriteFile(filePath, []byte("<html><body><div>etag-test</div></body></html>"), 0o644); err != nil {
		t.Fatalf("failed to write management asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d; body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	etag := rr.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header in management panel response")
	}
	cacheControl := rr.Header().Get("Cache-Control")
	if strings.Contains(cacheControl, "no-store") {
		t.Fatalf("expected Cache-Control without no-store when asset exists, got %q", cacheControl)
	}

	condReq := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	condReq.Header.Set("If-None-Match", etag)
	condRR := httptest.NewRecorder()
	server.engine.ServeHTTP(condRR, condReq)

	if condRR.Code != http.StatusNotModified {
		t.Fatalf("expected status %d for conditional request, got %d; body=%s", http.StatusNotModified, condRR.Code, condRR.Body.String())
	}
	if condRR.Body.Len() != 0 {
		t.Fatalf("expected empty body for 304 response, got %d bytes", condRR.Body.Len())
	}
}

func TestManagementPluginsRouteRegistered(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")

	server := newTestServer(t)
	enabled := true
	server.cfg.Plugins.Configs = map[string]proxyconfig.PluginInstanceConfig{
		"sample": {Enabled: &enabled, Priority: 4},
	}
	if errWrite := os.WriteFile(server.configFilePath, []byte("{}\n"), 0o600); errWrite != nil {
		t.Fatalf("failed to write config file: %v", errWrite)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/management/plugins", nil)
	req.Header.Set("Authorization", "Bearer test-management-key")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var payload struct {
		PluginsEnabled bool  `json:"plugins_enabled"`
		Plugins        []any `json:"plugins"`
	}
	if errUnmarshal := json.Unmarshal(rr.Body.Bytes(), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal response: %v body=%s", errUnmarshal, rr.Body.String())
	}
	if payload.Plugins == nil {
		t.Fatalf("plugins field = nil, want array; body=%s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/plugins/sample/config", nil)
	req.Header.Set("Authorization", "Bearer test-management-key")
	rr = httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var configPayload struct {
		Enabled  bool `json:"enabled"`
		Priority int  `json:"priority"`
	}
	if errUnmarshal := json.Unmarshal(rr.Body.Bytes(), &configPayload); errUnmarshal != nil {
		t.Fatalf("unmarshal config response: %v body=%s", errUnmarshal, rr.Body.String())
	}
	if !configPayload.Enabled || configPayload.Priority != 4 {
		t.Fatalf("plugin config = %#v, want enabled true priority 4", configPayload)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v0/management/plugins/sample", nil)
	req.Header.Set("Authorization", "Bearer test-management-key")
	rr = httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHomeEnabledHidesManagementEndpointsAndControlPanel(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")

	server := newTestServer(t)
	server.cfg.Home.Enabled = true

	t.Run("management endpoints return 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v0/management/config", nil)
		req.Header.Set("Authorization", "Bearer test-management-key")
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
		}
	})

	t.Run("management control panel returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/management.html", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
		}
	})
}

func TestAmpProviderModelRoutes(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
	}{
		{
			name:         "openai root models",
			path:         "/api/provider/openai/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "groq root models",
			path:         "/api/provider/groq/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "openai models",
			path:         "/api/provider/openai/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "anthropic models",
			path:         "/api/provider/anthropic/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"data"`,
		},
		{
			name:         "google models v1",
			path:         "/api/provider/google/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
		{
			name:         "google models v1beta",
			path:         "/api/provider/google/v1beta/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer test-key")

			rr := httptest.NewRecorder()
			server.engine.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("unexpected status code for %s: got %d want %d; body=%s", tc.path, rr.Code, tc.wantStatus, rr.Body.String())
			}
			if body := rr.Body.String(); !strings.Contains(body, tc.wantContains) {
				t.Fatalf("response body for %s missing %q: %s", tc.path, tc.wantContains, body)
			}
		})
	}
}

func TestModelsWithClientVersionReturnsCodexCatalog(t *testing.T) {
	modelRegistry := registry.GetGlobalRegistry()
	clientID := "test-client-version-catalog"
	modelRegistry.RegisterClient(clientID, "openai", []*registry.ModelInfo{
		{
			ID:            "gpt-5.5",
			Object:        "model",
			Created:       1776902400,
			OwnedBy:       "openai",
			Type:          "openai",
			DisplayName:   "GPT 5.5",
			Description:   "Frontier model for complex coding, research, and real-world work.",
			ContextLength: 272000,
			Thinking:      &registry.ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh"}},
		},
		{
			ID:            "custom-codex-model-test",
			Object:        "model",
			OwnedBy:       "test",
			Type:          "openai",
			DisplayName:   "Custom Codex Model",
			Description:   "Custom model from registry",
			ContextLength: 123456,
			Thinking:      &registry.ThinkingSupport{Levels: []string{"none", "minimal", "low", "medium", "unsupported", "high", "xhigh"}},
		},
		{ID: "grok-imagine-image-quality", Object: "model", OwnedBy: "xai", Type: "openai"},
		{ID: "gpt-image-2", Object: "model", OwnedBy: "openai", Type: "openai"},
		{ID: "grok-imagine-image", Object: "model", OwnedBy: "xai", Type: "openai"},
		{ID: "grok-imagine-video", Object: "model", OwnedBy: "xai", Type: "openai"},
		{ID: "grok-imagine-video-1.5-preview", Object: "model", OwnedBy: "xai", Type: "openai"},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(clientID)
	})

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/models?client_version", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("User-Agent", "claude-cli/1.0")

	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Models []map[string]any `json:"models"`
		Object string           `json:"object"`
		Data   []any            `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v; body=%s", err, rr.Body.String())
	}
	if resp.Object != "" || resp.Data != nil {
		t.Fatalf("expected codex catalog format without object/data, got object=%q data=%v", resp.Object, resp.Data)
	}
	if len(resp.Models) == 0 {
		t.Fatal("expected codex catalog models")
	}

	var gpt55 map[string]any
	var custom map[string]any
	for _, model := range resp.Models {
		switch slug, _ := model["slug"].(string); slug {
		case "gpt-5.5":
			gpt55 = model
		case "custom-codex-model-test":
			custom = model
		}
	}
	if gpt55 == nil {
		t.Fatal("expected gpt-5.5 codex catalog entry")
	}
	if _, ok := gpt55["minimal_client_version"]; !ok {
		t.Fatal("expected minimal_client_version in codex catalog")
	}
	serviceTiers, ok := gpt55["service_tiers"].([]any)
	if !ok || len(serviceTiers) != 1 {
		t.Fatalf("expected gpt-5.5 priority service tier, got %#v", gpt55["service_tiers"])
	}
	if custom == nil {
		t.Fatal("expected custom model codex catalog entry")
	}
	if got, _ := custom["display_name"].(string); got != "Custom Codex Model" {
		t.Fatalf("custom display_name = %q, want Custom Codex Model", got)
	}
	if got, _ := custom["description"].(string); got != "Custom model from registry" {
		t.Fatalf("custom description = %q, want Custom model from registry", got)
	}
	if got, _ := custom["context_window"].(float64); got != 123456 {
		t.Fatalf("custom context_window = %v, want 123456", custom["context_window"])
	}
	assertCodexSupportedReasoningLevels(t, custom, []string{"none", "low", "medium", "high", "xhigh"})
	if custom["base_instructions"] != gpt55["base_instructions"] {
		t.Fatal("expected custom model to use gpt-5.5 base_instructions fallback")
	}
	if _, ok := custom["available_in_plans"].([]any); !ok {
		t.Fatalf("expected custom model to use gpt-5.5 available_in_plans fallback, got %#v", custom["available_in_plans"])
	}
	if got, _ := custom["prefer_websockets"].(bool); got {
		t.Fatalf("custom prefer_websockets = %v, want false", custom["prefer_websockets"])
	}
	if _, ok := custom["apply_patch_tool_type"]; ok {
		t.Fatal("expected custom model to omit apply_patch_tool_type")
	}
	if _, ok := custom["upgrade"]; ok {
		t.Fatal("expected custom model to omit upgrade")
	}
	if _, ok := custom["availability_nux"]; ok {
		t.Fatal("expected custom model to omit availability_nux")
	}

	hiddenModels := map[string]bool{
		"grok-imagine-image-quality":     false,
		"gpt-image-2":                    false,
		"grok-imagine-image":             false,
		"grok-imagine-video":             false,
		"grok-imagine-video-1.5-preview": false,
	}
	for _, model := range resp.Models {
		slug, _ := model["slug"].(string)
		if _, ok := hiddenModels[slug]; !ok {
			continue
		}
		if visibility, _ := model["visibility"].(string); visibility != "hide" {
			t.Fatalf("%s visibility = %q, want hide", slug, visibility)
		}
		hiddenModels[slug] = true
	}
	for slug, found := range hiddenModels {
		if !found {
			t.Fatalf("expected hidden model %s in codex catalog", slug)
		}
	}
}

func assertCodexSupportedReasoningLevels(t *testing.T, model map[string]any, want []string) {
	t.Helper()

	rawLevels, ok := model["supported_reasoning_levels"].([]any)
	if !ok {
		t.Fatalf("expected supported_reasoning_levels, got %#v", model["supported_reasoning_levels"])
	}
	if len(rawLevels) != len(want) {
		t.Fatalf("supported_reasoning_levels length = %d, want %d: %#v", len(rawLevels), len(want), rawLevels)
	}
	for index, rawLevel := range rawLevels {
		levelEntry, ok := rawLevel.(map[string]any)
		if !ok {
			t.Fatalf("supported_reasoning_levels[%d] = %#v, want object", index, rawLevel)
		}
		if got, _ := levelEntry["effort"].(string); got != want[index] {
			t.Fatalf("supported_reasoning_levels[%d].effort = %q, want %q", index, got, want[index])
		}
	}
}

func TestDefaultRequestLoggerFactory_UsesResolvedLogDirectory(t *testing.T) {
	t.Setenv("WRITABLE_PATH", "")
	t.Setenv("writable_path", "")

	originalWD, errGetwd := os.Getwd()
	if errGetwd != nil {
		t.Fatalf("failed to get current working directory: %v", errGetwd)
	}

	tmpDir := t.TempDir()
	if errChdir := os.Chdir(tmpDir); errChdir != nil {
		t.Fatalf("failed to switch working directory: %v", errChdir)
	}
	defer func() {
		if errChdirBack := os.Chdir(originalWD); errChdirBack != nil {
			t.Fatalf("failed to restore working directory: %v", errChdirBack)
		}
	}()

	// Force ResolveLogDirectory to fallback to auth-dir/logs by making ./logs not a writable directory.
	if errWriteFile := os.WriteFile(filepath.Join(tmpDir, "logs"), []byte("not-a-directory"), 0o644); errWriteFile != nil {
		t.Fatalf("failed to create blocking logs file: %v", errWriteFile)
	}

	configDir := filepath.Join(tmpDir, "config")
	if errMkdirConfig := os.MkdirAll(configDir, 0o755); errMkdirConfig != nil {
		t.Fatalf("failed to create config dir: %v", errMkdirConfig)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	authDir := filepath.Join(tmpDir, "auth")
	if errMkdirAuth := os.MkdirAll(authDir, 0o700); errMkdirAuth != nil {
		t.Fatalf("failed to create auth dir: %v", errMkdirAuth)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			RequestLog: false,
		},
		AuthDir:           authDir,
		ErrorLogsMaxFiles: 10,
	}

	logger := defaultRequestLoggerFactory(cfg, configPath)
	fileLogger, ok := logger.(*internallogging.FileRequestLogger)
	if !ok {
		t.Fatalf("expected *FileRequestLogger, got %T", logger)
	}

	errLog := fileLogger.LogRequestWithOptions(
		"/v1/chat/completions",
		http.MethodPost,
		map[string][]string{"Content-Type": []string{"application/json"}},
		[]byte(`{"input":"hello"}`),
		http.StatusBadGateway,
		map[string][]string{"Content-Type": []string{"application/json"}},
		[]byte(`{"error":"upstream failure"}`),
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		"issue-1711",
		time.Now(),
		time.Now(),
	)
	if errLog != nil {
		t.Fatalf("failed to write forced error request log: %v", errLog)
	}

	authLogsDir := filepath.Join(authDir, "logs")
	authEntries, errReadAuthDir := os.ReadDir(authLogsDir)
	if errReadAuthDir != nil {
		t.Fatalf("failed to read auth logs dir %s: %v", authLogsDir, errReadAuthDir)
	}
	foundErrorLogInAuthDir := false
	for _, entry := range authEntries {
		if strings.HasPrefix(entry.Name(), "error-") && strings.HasSuffix(entry.Name(), ".log") {
			foundErrorLogInAuthDir = true
			break
		}
	}
	if !foundErrorLogInAuthDir {
		t.Fatalf("expected forced error log in auth fallback dir %s, got entries: %+v", authLogsDir, authEntries)
	}

	configLogsDir := filepath.Join(configDir, "logs")
	configEntries, errReadConfigDir := os.ReadDir(configLogsDir)
	if errReadConfigDir != nil && !os.IsNotExist(errReadConfigDir) {
		t.Fatalf("failed to inspect config logs dir %s: %v", configLogsDir, errReadConfigDir)
	}
	for _, entry := range configEntries {
		if strings.HasPrefix(entry.Name(), "error-") && strings.HasSuffix(entry.Name(), ".log") {
			t.Fatalf("unexpected forced error log in config dir %s", configLogsDir)
		}
	}
}

func TestCodexRefreshActionTokenAuthorizesScopedEndpoints(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")
	server := newTestServer(t)
	if server.mgmt == nil {
		t.Fatal("expected management handler")
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/v0/management/codex-refresh-auth-files", nil)
	missingRR := httptest.NewRecorder()
	server.engine.ServeHTTP(missingRR, missingReq)
	if missingRR.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d body=%s", missingRR.Code, http.StatusUnauthorized, missingRR.Body.String())
	}

	token := server.mgmt.SignCodexRefreshActionToken()
	if token == "" {
		t.Fatal("expected non-empty codex refresh token")
	}

	authFilesReq := httptest.NewRequest(http.MethodGet, "/v0/management/codex-refresh-auth-files", nil)
	authFilesReq.Header.Set("X-Codex-Refresh-Token", token)
	authFilesRR := httptest.NewRecorder()
	server.engine.ServeHTTP(authFilesRR, authFilesReq)
	if authFilesRR.Code != http.StatusOK {
		t.Fatalf("scoped auth-files status = %d body=%s", authFilesRR.Code, authFilesRR.Body.String())
	}

	configReq := httptest.NewRequest(http.MethodGet, "/v0/management/config", nil)
	configReq.Header.Set("X-Codex-Refresh-Token", token)
	configRR := httptest.NewRecorder()
	server.engine.ServeHTTP(configRR, configReq)
	if configRR.Code != http.StatusUnauthorized {
		t.Fatalf("scoped token should not authorize config endpoint, got status = %d body=%s", configRR.Code, configRR.Body.String())
	}
}

func codexRefreshTokenFromPanelBody(t *testing.T, body string) string {
	t.Helper()
	prefix := `var CODEX_REFRESH_TOKEN = "`
	start := strings.Index(body, prefix)
	if start < 0 {
		t.Fatalf("expected panel body to contain codex refresh token assignment, got %s", body)
	}
	start += len(prefix)
	end := strings.Index(body[start:], `";`)
	if end < 0 {
		t.Fatalf("expected panel body to contain terminated codex refresh token assignment, got %s", body)
	}
	return body[start : start+end]
}

func TestServeManagementControlPanel_CodexRefreshTokenRequiresManagementAuth(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")
	server := newTestServer(t)

	staticDir := filepath.Join(filepath.Dir(server.configFilePath), "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}
	filePath := filepath.Join(staticDir, "management.html")
	if err := os.WriteFile(filePath, []byte("<html><body><div>ok</div></body></html>"), 0o644); err != nil {
		t.Fatalf("failed to write management asset: %v", err)
	}

	unauthReq := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	unauthRR := httptest.NewRecorder()
	server.engine.ServeHTTP(unauthRR, unauthReq)
	if unauthRR.Code != http.StatusOK {
		t.Fatalf("unauthenticated panel status = %d body=%s", unauthRR.Code, unauthRR.Body.String())
	}
	unauthToken := codexRefreshTokenFromPanelBody(t, unauthRR.Body.String())
	if unauthToken == "" {
		t.Fatalf("expected panel to contain a scoped codex refresh token")
	}
	unauthAuthFilesReq := httptest.NewRequest(http.MethodGet, "/v0/management/codex-refresh-auth-files", nil)
	unauthAuthFilesReq.Header.Set("X-Codex-Refresh-Token", unauthToken)
	unauthAuthFilesRR := httptest.NewRecorder()
	server.engine.ServeHTTP(unauthAuthFilesRR, unauthAuthFilesReq)
	if unauthAuthFilesRR.Code != http.StatusOK {
		t.Fatalf("panel scoped token auth-files status = %d body=%s", unauthAuthFilesRR.Code, unauthAuthFilesRR.Body.String())
	}
	if len(unauthRR.Result().Cookies()) != 0 {
		t.Fatalf("expected unauthenticated panel not to issue cookies, got %+v", unauthRR.Result().Cookies())
	}

	panelReq := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	panelReq.Header.Set("Authorization", "Bearer test-management-key")
	panelRR := httptest.NewRecorder()
	server.engine.ServeHTTP(panelRR, panelReq)
	if panelRR.Code != http.StatusOK {
		t.Fatalf("authenticated panel status = %d body=%s", panelRR.Code, panelRR.Body.String())
	}
	if strings.Contains(panelRR.Body.String(), `var CODEX_REFRESH_TOKEN = "";`) {
		t.Fatalf("expected authenticated panel to contain a non-empty codex refresh token")
	}

	sessionCookies := make(map[string]*http.Cookie)
	for _, cookie := range panelRR.Result().Cookies() {
		if cookie.Name == "cpa_management_session" {
			sessionCookies[cookie.Path] = cookie
		}
	}
	apiSessionCookie := sessionCookies["/v0/management"]
	panelSessionCookie := sessionCookies["/management.html"]
	if apiSessionCookie == nil {
		t.Fatalf("expected /management.html to issue /v0/management session cookie")
	}
	if panelSessionCookie == nil {
		t.Fatalf("expected /management.html to issue /management.html session cookie")
	}
	if apiSessionCookie.Value == "" || apiSessionCookie.Value != panelSessionCookie.Value {
		t.Fatalf("expected management session cookies to have the same non-empty value")
	}
	if apiSessionCookie.HttpOnly != panelSessionCookie.HttpOnly || !apiSessionCookie.HttpOnly {
		t.Fatalf("expected management session cookies to be HttpOnly")
	}
	if apiSessionCookie.SameSite != panelSessionCookie.SameSite || apiSessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected management session cookies to use SameSite=Lax")
	}
	if apiSessionCookie.Secure != panelSessionCookie.Secure {
		t.Fatalf("expected management session cookies to use matching Secure attributes")
	}
	if !apiSessionCookie.Expires.Equal(panelSessionCookie.Expires) {
		t.Fatalf("expected management session cookies to use matching Expires attributes")
	}
	if apiSessionCookie.MaxAge != panelSessionCookie.MaxAge {
		t.Fatalf("expected management session cookies to use matching MaxAge attributes")
	}

	sessionPanelReq := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	sessionPanelReq.AddCookie(panelSessionCookie)
	sessionPanelRR := httptest.NewRecorder()
	server.engine.ServeHTTP(sessionPanelRR, sessionPanelReq)
	if sessionPanelRR.Code != http.StatusOK {
		t.Fatalf("session panel status = %d body=%s", sessionPanelRR.Code, sessionPanelRR.Body.String())
	}
	if strings.Contains(sessionPanelRR.Body.String(), `var CODEX_REFRESH_TOKEN = "";`) {
		t.Fatalf("expected session-authenticated panel to contain a non-empty codex refresh token")
	}

	authFilesReq := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
	authFilesReq.AddCookie(apiSessionCookie)
	authFilesRR := httptest.NewRecorder()
	server.engine.ServeHTTP(authFilesRR, authFilesReq)
	if authFilesRR.Code != http.StatusOK {
		t.Fatalf("auth-files with session cookie status = %d body=%s", authFilesRR.Code, authFilesRR.Body.String())
	}
}

func TestServeManagementControlPanel_DisablesCaching(t *testing.T) {
	server := newTestServer(t)
	server.cfg.RemoteManagement.SecretKey = "test"

	staticDir := filepath.Join(filepath.Dir(server.configFilePath), "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}
	filePath := filepath.Join(staticDir, "management.html")
	if err := os.WriteFile(filePath, []byte("<html><body><div>ok</div></body></html>"), 0o644); err != nil {
		t.Fatalf("failed to write management asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d; body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	cacheControl := strings.ToLower(rr.Header().Get("Cache-Control"))
	if !strings.Contains(cacheControl, "no-cache") {
		t.Fatalf("expected Cache-Control to contain no-cache, got %q", rr.Header().Get("Cache-Control"))
	}
	etag := rr.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header in management panel response")
	}
	body := rr.Body.String()
	if !strings.Contains(body, "__cpa_model_price_dropdown_clip_patch__") {
		t.Fatalf("expected dropdown patch marker in management response, got %s", body)
	}
	if !strings.Contains(body, "__cpa_codex_free_refresh_patch__") {
		t.Fatalf("expected codex free refresh patch marker in management response, got %s", body)
	}
	if strings.Contains(body, "sessionStorage") || strings.Contains(body, "localStorage") || strings.Contains(body, "meta[name") || strings.Contains(body, `name="management-key"`) {
		t.Fatalf("expected codex refresh patch not to expose management auth material via DOM or browser storage, got %s", body)
	}
	if !strings.Contains(body, "X-Codex-Refresh-Token") || strings.Contains(body, "captureMgmtHeaders") || strings.Contains(body, "X-Management-Key") {
		t.Fatalf("expected codex refresh patch to use only scoped action token auth, got %s", body)
	}
	if !strings.Contains(body, "auth_index") || !strings.Contains(body, "codex-single-refresh-btn") {
		t.Fatalf("expected codex single refresh patch code in management response, got %s", body)
	}
	if !strings.Contains(body, `credentials: "same-origin"`) {
		t.Fatalf("expected codex refresh patch to include same-origin credentials, got %s", body)
	}
	if !strings.Contains(body, "fetchAuthFilesAttempt") || !strings.Contains(body, "r.status === 401") {
		t.Fatalf("expected codex refresh patch to retry auth-files after temporary 401, got %s", body)
	}
	if !strings.Contains(body, "decodeURIComponent") || !strings.Contains(body, "auth files") || !strings.Contains(body, "/auth") || !strings.Contains(body, "认证文件") || !strings.Contains(body, "凭证") {
		t.Fatalf("expected auth route detection tokens in codex refresh patch, got %s", body)
	}
	if !strings.Contains(body, "activeAuthRouteFromNavigation") || !strings.Contains(body, "[aria-current='page']") || !strings.Contains(body, "[aria-selected='true']") || !strings.Contains(body, "[role='tab']") {
		t.Fatalf("expected codex refresh patch to recognize active Auth Files navigation when URL is generic, got %s", body)
	}
	if !strings.Contains(body, "[data-state='active']") || !strings.Contains(body, "main [class*='auth']") || !strings.Contains(body, "main [id*='auth']") {
		t.Fatalf("expected codex refresh patch to preserve known-good auth page fallbacks, got %s", body)
	}
	if !strings.Contains(body, "removeInjectedUI(); return") || !strings.Contains(body, "setTimeout(scheduleAuthPatch, 2500)") || !strings.Contains(body, "replaceState") {
		t.Fatalf("expected codex refresh patch to clean up injected UI across route changes, got %s", body)
	}
	if !strings.Contains(body, "getAuthFiles(status).then(function (files) {\n      if (!isAuthRoute()) { removeInjectedUI(); return; }") {
		t.Fatalf("expected codex single refresh injection to re-check auth route after async auth file load, got %s", body)
	}
	if !strings.Contains(body, "var scopes = section ? [section, document] : [document]") {
		t.Fatalf("expected codex single refresh injection to search document when auth section is too narrow, got %s", body)
	}
	if strings.Contains(body, "__cpa_api_key_usage_dashboard_patch__") {
		t.Fatalf("expected API key usage dashboard patch marker to be absent from management response, got %s", body)
	}
}
