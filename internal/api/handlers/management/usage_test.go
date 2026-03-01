package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	usagepkg "github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

type usageResponse struct {
	Usage       usagepkg.StatisticsSnapshot `json:"usage"`
	DetailsMode string                      `json:"details_mode"`
	WindowHours int                         `json:"window_hours"`
}

func TestGetUsageStatistics_DefaultIncludesDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := usagepkg.NewRequestStatistics()
	usagepkg.SetStatisticsEnabled(true)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 5,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp usageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.DetailsMode != "all" {
		t.Fatalf("expected details_mode=all, got %q", resp.DetailsMode)
	}

	model := resp.Usage.APIs["api-1"].Models["model-1"]
	if len(model.Details) != 1 {
		t.Fatalf("expected details in default response, got %d", len(model.Details))
	}
	if model.InputTokens != 10 || model.OutputTokens != 5 {
		t.Fatalf("expected aggregated tokens to be preserved, got input=%d output=%d", model.InputTokens, model.OutputTokens)
	}
}

func TestGetUsageStatistics_SupportsDetailsLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := usagepkg.NewRequestStatistics()
	usagepkg.SetStatisticsEnabled(true)
	base := time.Date(2026, 2, 20, 11, 0, 0, 0, time.UTC)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: base,
		Detail: coreusage.Detail{
			InputTokens:  3,
			OutputTokens: 2,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: base.Add(1 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:  4,
			OutputTokens: 1,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?details=1&detail_limit=1", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp usageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.DetailsMode != "limited" {
		t.Fatalf("expected details_mode=limited, got %q", resp.DetailsMode)
	}

	model := resp.Usage.APIs["api-1"].Models["model-1"]
	if len(model.Details) != 1 {
		t.Fatalf("expected one detail record, got %d", len(model.Details))
	}
	if !model.Details[0].Timestamp.Equal(base.Add(1 * time.Minute)) {
		t.Fatalf("expected latest detail timestamp, got %s", model.Details[0].Timestamp)
	}
}

func TestGetUsageStatistics_CompactOmitsDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := usagepkg.NewRequestStatistics()
	usagepkg.SetStatisticsEnabled(true)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  9,
			OutputTokens: 3,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?compact=1", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp usageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.DetailsMode != "none" {
		t.Fatalf("expected details_mode=none, got %q", resp.DetailsMode)
	}

	model := resp.Usage.APIs["api-1"].Models["model-1"]
	if len(model.Details) != 0 {
		t.Fatalf("expected no details in compact response, got %d", len(model.Details))
	}
	if model.InputTokens != 9 || model.OutputTokens != 3 {
		t.Fatalf("expected aggregated tokens to be preserved, got input=%d output=%d", model.InputTokens, model.OutputTokens)
	}
}

func TestGetUsageStatistics_WindowHoursFiltersData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := usagepkg.NewRequestStatistics()
	usagepkg.SetStatisticsEnabled(true)

	now := time.Now().UTC()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: now.Add(-2 * time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  7,
			OutputTokens: 3,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: now.Add(-30 * time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  5,
			OutputTokens: 2,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?window_hours=24&details=1", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp usageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.WindowHours != 24 {
		t.Fatalf("expected window_hours=24, got %d", resp.WindowHours)
	}
	if resp.Usage.TotalRequests != 1 {
		t.Fatalf("expected filtered total_requests=1, got %d", resp.Usage.TotalRequests)
	}
	if resp.Usage.TotalTokens != 10 {
		t.Fatalf("expected filtered total_tokens=10, got %d", resp.Usage.TotalTokens)
	}

	model := resp.Usage.APIs["api-1"].Models["model-1"]
	if model.TotalRequests != 1 || len(model.Details) != 1 {
		t.Fatalf("expected filtered model data with one detail, got requests=%d details=%d", model.TotalRequests, len(model.Details))
	}
}
