package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

type usageExportPayload struct {
	Version    int                      `json:"version"`
	ExportedAt time.Time                `json:"exported_at"`
	Usage      usage.StatisticsSnapshot `json:"usage"`
}

type usageImportPayload struct {
	Version int                      `json:"version"`
	Usage   usage.StatisticsSnapshot `json:"usage"`
}

type usageSnapshotCacheEntry struct {
	snapshot usage.StatisticsSnapshot
	expires  time.Time
}

type usagePagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

type usageRequestDetailItem struct {
	APIKey    string           `json:"api_key"`
	Model     string           `json:"model"`
	Timestamp time.Time        `json:"timestamp"`
	Source    string           `json:"source"`
	AuthIndex string           `json:"auth_index"`
	Failed    bool             `json:"failed"`
	Tokens    usage.TokenStats `json:"tokens"`
}

type usageRequestDetailsPage struct {
	Items      []usageRequestDetailItem `json:"items"`
	Pagination usagePagination          `json:"pagination"`
}

const usageSnapshotCacheTTL = 20 * time.Second

// GetUsageStatistics returns the in-memory request statistics snapshot.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
	detailLimit := parseUsageDetailLimit(c)
	windowHours := parseUsageWindowHours(c)
	apiPage, apiPageSize := parseUsagePaginationParams(c, "api_page", "api_page_size")
	detailPage, detailPageSize := parseUsagePaginationParams(c, "detail_page", "detail_page_size")

	snapshot := h.resolveUsageSnapshot(windowHours, detailLimit)
	apiPagination := usagePagination{}
	if apiPageSize > 0 {
		snapshot, apiPagination = paginateUsageAPIs(snapshot, apiPage, apiPageSize)
	}

	requestDetailsPage := usageRequestDetailsPage{}
	if detailPageSize > 0 {
		fullDetailsSnapshot := h.resolveUsageSnapshot(windowHours, -1)
		requestDetailsPage = buildUsageRequestDetailsPage(fullDetailsSnapshot, detailPage, detailPageSize)
	}

	c.JSON(http.StatusOK, gin.H{
		"usage":                 snapshot,
		"failed_requests":       snapshot.FailureCount,
		"details_mode":          usageDetailsMode(detailLimit),
		"window_hours":          windowHours,
		"api_pagination":        apiPagination,
		"request_details_page":  requestDetailsPage,
	})
}

func (h *Handler) resolveUsageSnapshot(windowHours, detailLimit int) usage.StatisticsSnapshot {
	var snapshot usage.StatisticsSnapshot
	cacheKey := usageSnapshotCacheKey(windowHours, detailLimit)
	if cached, ok := h.getUsageSnapshotFromCache(cacheKey); ok {
		return cached
	}
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.SnapshotRecentWithDetailLimit(windowHours, detailLimit)
		h.storeUsageSnapshotCache(cacheKey, snapshot)
	}
	return snapshot
}

func usageSnapshotCacheKey(windowHours, detailLimit int) string {
	return fmt.Sprintf("w:%d|d:%d", windowHours, detailLimit)
}

func parseUsagePaginationParams(c *gin.Context, pageKey, sizeKey string) (int, int) {
	if c == nil {
		return 1, 0
	}
	rawPage := strings.TrimSpace(c.Query(pageKey))
	rawSize := strings.TrimSpace(c.Query(sizeKey))
	if rawPage == "" && rawSize == "" {
		return 1, 0
	}

	page := 1
	if rawPage != "" {
		if parsed, err := strconv.Atoi(rawPage); err == nil && parsed > 0 {
			page = parsed
		}
	}

	pageSize := 50
	if rawSize != "" {
		if parsed, err := strconv.Atoi(rawSize); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func buildUsagePagination(totalItems, page, pageSize int) usagePagination {
	if pageSize <= 0 {
		return usagePagination{}
	}
	if page < 1 {
		page = 1
	}
	totalPages := 0
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
		if page > totalPages {
			page = totalPages
		}
	} else {
		page = 1
	}
	return usagePagination{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}

func paginateUsageAPIs(snapshot usage.StatisticsSnapshot, page, pageSize int) (usage.StatisticsSnapshot, usagePagination) {
	if pageSize <= 0 {
		return snapshot, usagePagination{}
	}
	apiKeys := make([]string, 0, len(snapshot.APIs))
	for apiKey := range snapshot.APIs {
		apiKeys = append(apiKeys, apiKey)
	}
	sort.Slice(apiKeys, func(i, j int) bool {
		left := snapshot.APIs[apiKeys[i]]
		right := snapshot.APIs[apiKeys[j]]
		if left.TotalRequests != right.TotalRequests {
			return left.TotalRequests > right.TotalRequests
		}
		if left.TotalTokens != right.TotalTokens {
			return left.TotalTokens > right.TotalTokens
		}
		return apiKeys[i] < apiKeys[j]
	})

	pagination := buildUsagePagination(len(apiKeys), page, pageSize)
	if len(apiKeys) == 0 || pagination.TotalPages == 0 {
		snapshot.APIs = map[string]usage.APISnapshot{}
		return snapshot, pagination
	}
	start := (pagination.Page - 1) * pagination.PageSize
	end := start + pagination.PageSize
	if end > len(apiKeys) {
		end = len(apiKeys)
	}
	trimmed := make(map[string]usage.APISnapshot, end-start)
	for _, apiKey := range apiKeys[start:end] {
		trimmed[apiKey] = snapshot.APIs[apiKey]
	}
	snapshot.APIs = trimmed
	return snapshot, pagination
}

func buildUsageRequestDetailsPage(snapshot usage.StatisticsSnapshot, page, pageSize int) usageRequestDetailsPage {
	rows := make([]usageRequestDetailItem, 0)
	for apiKey, apiSnapshot := range snapshot.APIs {
		for modelName, modelSnapshot := range apiSnapshot.Models {
			for _, detail := range modelSnapshot.Details {
				rows = append(rows, usageRequestDetailItem{
					APIKey:    apiKey,
					Model:     modelName,
					Timestamp: detail.Timestamp,
					Source:    detail.Source,
					AuthIndex: detail.AuthIndex,
					Failed:    detail.Failed,
					Tokens:    detail.Tokens,
				})
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Timestamp.Equal(rows[j].Timestamp) {
			return rows[i].Timestamp.After(rows[j].Timestamp)
		}
		if rows[i].APIKey != rows[j].APIKey {
			return rows[i].APIKey < rows[j].APIKey
		}
		if rows[i].Model != rows[j].Model {
			return rows[i].Model < rows[j].Model
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].AuthIndex < rows[j].AuthIndex
	})

	pagination := buildUsagePagination(len(rows), page, pageSize)
	if len(rows) == 0 || pagination.TotalPages == 0 {
		return usageRequestDetailsPage{Items: []usageRequestDetailItem{}, Pagination: pagination}
	}
	start := (pagination.Page - 1) * pagination.PageSize
	end := start + pagination.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	return usageRequestDetailsPage{
		Items:      rows[start:end],
		Pagination: pagination,
	}
}

func (h *Handler) getUsageSnapshotFromCache(key string) (usage.StatisticsSnapshot, bool) {
	if h == nil {
		return usage.StatisticsSnapshot{}, false
	}
	h.usageCacheMu.Lock()
	defer h.usageCacheMu.Unlock()
	entry, ok := h.usageCache[key]
	if !ok {
		return usage.StatisticsSnapshot{}, false
	}
	if time.Now().After(entry.expires) {
		delete(h.usageCache, key)
		return usage.StatisticsSnapshot{}, false
	}
	return entry.snapshot, true
}

func (h *Handler) storeUsageSnapshotCache(key string, snapshot usage.StatisticsSnapshot) {
	if h == nil {
		return
	}
	h.usageCacheMu.Lock()
	h.usageCache[key] = usageSnapshotCacheEntry{
		snapshot: snapshot,
		expires:  time.Now().Add(usageSnapshotCacheTTL),
	}
	h.usageCacheMu.Unlock()
}

func (h *Handler) clearUsageSnapshotCache() {
	if h == nil {
		return
	}
	h.usageCacheMu.Lock()
	h.usageCache = make(map[string]usageSnapshotCacheEntry)
	h.usageCacheMu.Unlock()
}

func parseUsageDetailLimit(c *gin.Context) int {
	if c == nil {
		return -1
	}
	if isTruthy(c.Query("compact")) {
		return 0
	}
	detailsRaw, detailsProvided := c.GetQuery("details")
	if detailsProvided && !isTruthy(detailsRaw) {
		return 0
	}

	rawLimit := strings.TrimSpace(c.Query("detail_limit"))
	if rawLimit == "" {
		// Keep backward compatibility: default response includes full details.
		return -1
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return -1
	}
	if limit < 0 {
		return -1
	}
	return limit
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func usageDetailsMode(detailLimit int) string {
	switch {
	case detailLimit == 0:
		return "none"
	case detailLimit < 0:
		return "all"
	default:
		return "limited"
	}
}

func parseUsageWindowHours(c *gin.Context) int {
	if c == nil {
		return 0
	}
	raw := strings.TrimSpace(c.Query("window_hours"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("hours"))
	}
	if raw == "" {
		return 0
	}
	raw = strings.TrimSuffix(strings.ToLower(raw), "h")
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	maxHours := 24 * 365
	if value > maxHours {
		return maxHours
	}
	return value
}

// ExportUsageStatistics returns a complete usage snapshot for backup/migration.
func (h *Handler) ExportUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	})
}

// ImportUsageStatistics merges a previously exported usage snapshot into memory.
func (h *Handler) ImportUsageStatistics(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload usageImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if payload.Version != 0 && payload.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
		return
	}

	result := h.usageStats.MergeSnapshot(payload.Usage)
	h.clearUsageSnapshotCache()
	snapshot := h.usageStats.SnapshotWithDetailLimit(0)
	c.JSON(http.StatusOK, gin.H{
		"added":           result.Added,
		"skipped":         result.Skipped,
		"total_requests":  snapshot.TotalRequests,
		"failed_requests": snapshot.FailureCount,
	})
}
