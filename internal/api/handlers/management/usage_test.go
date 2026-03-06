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
	Usage              usagepkg.StatisticsSnapshot  `json:"usage"`
	DetailsMode        string                       `json:"details_mode"`
	WindowHours        int                          `json:"window_hours"`
	APIPagination      usagePaginationResponse     `json:"api_pagination"`
	RequestDetailsPage usageRequestDetailsResponse `json:"request_details_page"`
}

type usagePaginationResponse struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

type usageRequestDetailsResponse struct {
	Items      []usageRequestDetailItem `json:"items"`
	Pagination usagePaginationResponse  `json:"pagination"`
}

type usageRequestDetailItem struct {
	APIKey    string              `json:"api_key"`
	Model     string              `json:"model"`
	Timestamp time.Time           `json:"timestamp"`
	Source    string              `json:"source"`
	AuthIndex string              `json:"auth_index"`
	Failed    bool                `json:"failed"`
	Tokens    usagepkg.TokenStats `json:"tokens"`
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

func TestGetUsageStatistics_SupportsAPIPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := usagepkg.NewRequestStatistics()
	usagepkg.SetStatisticsEnabled(true)
	base := time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC)

	for idx, apiKey := range []string{"api-1", "api-2", "api-3"} {
		stats.Record(context.Background(), coreusage.Record{
			APIKey:      apiKey,
			Model:       "model-1",
			RequestedAt: base.Add(time.Duration(idx) * time.Minute),
			Detail: coreusage.Detail{
				InputTokens:  int64((idx + 1) * 10),
				OutputTokens: int64((idx + 1) * 5),
			},
		})
	}

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?compact=1&api_page=2&api_page_size=1", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp usageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.APIPagination.Page != 2 || resp.APIPagination.PageSize != 1 {
		t.Fatalf("expected api_pagination page=2 size=1, got page=%d size=%d", resp.APIPagination.Page, resp.APIPagination.PageSize)
	}
	if resp.APIPagination.TotalItems != 3 || resp.APIPagination.TotalPages != 3 {
		t.Fatalf("expected api_pagination total_items=3 total_pages=3, got items=%d pages=%d", resp.APIPagination.TotalItems, resp.APIPagination.TotalPages)
	}
	if len(resp.Usage.APIs) != 1 {
		t.Fatalf("expected one visible API in paginated usage snapshot, got %d", len(resp.Usage.APIs))
	}
	if _, ok := resp.Usage.APIs["api-2"]; !ok {
		t.Fatalf("expected second API page to expose api-2, got %#v", resp.Usage.APIs)
	}
}

func TestGetUsageStatistics_SupportsRequestDetailPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := usagepkg.NewRequestStatistics()
	usagepkg.SetStatisticsEnabled(true)
	base := time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-a",
		RequestedAt: base,
		Source:      "openai",
		AuthIndex:   "0",
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 5,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-2",
		Model:       "model-b",
		RequestedAt: base.Add(2 * time.Minute),
		Source:      "claude",
		AuthIndex:   "1",
		Detail: coreusage.Detail{
			InputTokens:  7,
			OutputTokens: 3,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-c",
		RequestedAt: base.Add(1 * time.Minute),
		Source:      "gemini",
		AuthIndex:   "2",
		Detail: coreusage.Detail{
			InputTokens:  4,
			OutputTokens: 6,
		},
	})

	h := &Handler{usageStats: stats}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?compact=1&detail_page=1&detail_page_size=2", nil)

	h.GetUsageStatistics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp usageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.DetailsMode != "none" {
		t.Fatalf("expected compact response details_mode=none, got %q", resp.DetailsMode)
	}
	if resp.RequestDetailsPage.Pagination.Page != 1 || resp.RequestDetailsPage.Pagination.PageSize != 2 {
		t.Fatalf("expected request_details_page pagination page=1 size=2, got page=%d size=%d", resp.RequestDetailsPage.Pagination.Page, resp.RequestDetailsPage.Pagination.PageSize)
	}
	if resp.RequestDetailsPage.Pagination.TotalItems != 3 || resp.RequestDetailsPage.Pagination.TotalPages != 2 {
		t.Fatalf("expected request_details_page total_items=3 total_pages=2, got items=%d pages=%d", resp.RequestDetailsPage.Pagination.TotalItems, resp.RequestDetailsPage.Pagination.TotalPages)
	}
	if len(resp.RequestDetailsPage.Items) != 2 {
		t.Fatalf("expected first request detail page to contain two items, got %d", len(resp.RequestDetailsPage.Items))
	}
	if resp.RequestDetailsPage.Items[0].APIKey != "api-2" || resp.RequestDetailsPage.Items[0].Model != "model-b" {
		t.Fatalf("expected newest request detail first, got api=%q model=%q", resp.RequestDetailsPage.Items[0].APIKey, resp.RequestDetailsPage.Items[0].Model)
	}
	if resp.RequestDetailsPage.Items[1].APIKey != "api-1" || resp.RequestDetailsPage.Items[1].Model != "model-c" {
		t.Fatalf("expected second newest request detail second, got api=%q model=%q", resp.RequestDetailsPage.Items[1].APIKey, resp.RequestDetailsPage.Items[1].Model)
	}
}
