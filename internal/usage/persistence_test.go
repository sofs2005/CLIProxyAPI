package usage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestPersistenceRoundTrip(t *testing.T) {
	t.Cleanup(func() {
		_ = StopPersistence(context.Background())
		defaultRequestStatistics = NewRequestStatistics()
		persistenceDirty.Store(false)
	})

	defaultRequestStatistics = NewRequestStatistics()
	SetStatisticsEnabled(true)

	persistPath := filepath.Join(t.TempDir(), "usage", "usage.json")
	if err := StartPersistence(persistPath, 10*time.Millisecond); err != nil {
		t.Fatalf("StartPersistence() error = %v", err)
	}

	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	GetRequestStatistics().Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "test-model",
		RequestedAt: now,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 5,
		},
	})

	waitFor(t, 2*time.Second, func() bool {
		s := GetRequestStatistics().Snapshot()
		return s.TotalRequests == 1 && !persistenceDirty.Load()
	})

	if err := StopPersistence(context.Background()); err != nil {
		t.Fatalf("StopPersistence() error = %v", err)
	}

	defaultRequestStatistics = NewRequestStatistics()
	persistenceDirty.Store(false)

	if err := StartPersistence(persistPath, 10*time.Millisecond); err != nil {
		t.Fatalf("StartPersistence(reload) error = %v", err)
	}
	defer func() {
		_ = StopPersistence(context.Background())
	}()

	snapshot := GetRequestStatistics().Snapshot()
	if snapshot.TotalRequests != 1 {
		t.Fatalf("expected total_requests=1 after reload, got %d", snapshot.TotalRequests)
	}
	if snapshot.TotalTokens != 15 {
		t.Fatalf("expected total_tokens=15 after reload, got %d", snapshot.TotalTokens)
	}
	if snapshot.RequestsByHour["12"] != 1 {
		t.Fatalf("expected requests_by_hour[12]=1 after reload, got %d", snapshot.RequestsByHour["12"])
	}
	if snapshot.TokensByHour["12"] != 15 {
		t.Fatalf("expected tokens_by_hour[12]=15 after reload, got %d", snapshot.TokensByHour["12"])
	}
}

func TestMergeSnapshotAggregatesWithoutDetails(t *testing.T) {
	stats := NewRequestStatistics()
	snapshot := StatisticsSnapshot{
		TotalRequests: 7,
		SuccessCount:  6,
		FailureCount:  1,
		TotalTokens:   210,
		APIs: map[string]APISnapshot{
			"test-api": {
				TotalRequests: 7,
				TotalTokens:   210,
				Models: map[string]ModelSnapshot{
					"test-model": {
						TotalRequests:   7,
						TotalTokens:     210,
						InputTokens:     120,
						OutputTokens:    70,
						ReasoningTokens: 15,
						CachedTokens:    5,
					},
				},
			},
		},
		RequestsByDay: map[string]int64{
			"2026-02-19": 7,
		},
		RequestsByHour: map[string]int64{
			"09": 7,
		},
		TokensByDay: map[string]int64{
			"2026-02-19": 210,
		},
		TokensByHour: map[string]int64{
			"09": 210,
		},
	}

	result := stats.MergeSnapshot(snapshot)
	if result.Added != 0 {
		t.Fatalf("expected added=0 for aggregate-only snapshot, got %d", result.Added)
	}
	if result.Skipped != 0 {
		t.Fatalf("expected skipped=0 for aggregate-only snapshot, got %d", result.Skipped)
	}

	got := stats.Snapshot()
	if got.TotalRequests != 7 {
		t.Fatalf("expected total_requests=7, got %d", got.TotalRequests)
	}
	if got.TotalTokens != 210 {
		t.Fatalf("expected total_tokens=210, got %d", got.TotalTokens)
	}
	if got.RequestsByHour["09"] != 7 {
		t.Fatalf("expected requests_by_hour[09]=7, got %d", got.RequestsByHour["09"])
	}
	if got.TokensByHour["09"] != 210 {
		t.Fatalf("expected tokens_by_hour[09]=210, got %d", got.TokensByHour["09"])
	}

	apiSnapshot, ok := got.APIs["test-api"]
	if !ok {
		t.Fatal("expected api snapshot for test-api")
	}
	if apiSnapshot.TotalRequests != 7 || apiSnapshot.TotalTokens != 210 {
		t.Fatalf("unexpected api totals: requests=%d tokens=%d", apiSnapshot.TotalRequests, apiSnapshot.TotalTokens)
	}

	modelSnapshot, ok := apiSnapshot.Models["test-model"]
	if !ok {
		t.Fatal("expected model snapshot for test-model")
	}
	if modelSnapshot.TotalRequests != 7 || modelSnapshot.TotalTokens != 210 {
		t.Fatalf("unexpected model totals: requests=%d tokens=%d", modelSnapshot.TotalRequests, modelSnapshot.TotalTokens)
	}
	if modelSnapshot.InputTokens != 120 || modelSnapshot.OutputTokens != 70 || modelSnapshot.ReasoningTokens != 15 || modelSnapshot.CachedTokens != 5 {
		t.Fatalf(
			"unexpected model token breakdown: input=%d output=%d reasoning=%d cached=%d",
			modelSnapshot.InputTokens,
			modelSnapshot.OutputTokens,
			modelSnapshot.ReasoningTokens,
			modelSnapshot.CachedTokens,
		)
	}
}

func TestMergeSnapshotAggregatesWithoutDetailsOnNonEmptyStore(t *testing.T) {
	stats := NewRequestStatistics()
	SetStatisticsEnabled(true)
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "existing-api",
		Model:       "existing-model",
		RequestedAt: time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  3,
			OutputTokens: 2,
		},
	})

	snapshot := StatisticsSnapshot{
		TotalRequests: 7,
		SuccessCount:  6,
		FailureCount:  1,
		TotalTokens:   210,
		APIs: map[string]APISnapshot{
			"imported-api": {
				TotalRequests: 7,
				TotalTokens:   210,
				Models: map[string]ModelSnapshot{
					"imported-model": {
						TotalRequests: 7,
						TotalTokens:   210,
					},
				},
			},
		},
		RequestsByHour: map[string]int64{
			"09": 7,
		},
		TokensByHour: map[string]int64{
			"09": 210,
		},
	}

	result := stats.MergeSnapshot(snapshot)
	if result.Added != 0 || result.Skipped != 0 {
		t.Fatalf("expected no detail merge for aggregate-only snapshot, got added=%d skipped=%d", result.Added, result.Skipped)
	}

	got := stats.Snapshot()
	if got.TotalRequests != 1 {
		t.Fatalf("expected existing total_requests=1 to remain unchanged, got %d", got.TotalRequests)
	}
	if got.TotalTokens != 5 {
		t.Fatalf("expected existing total_tokens=5 to remain unchanged, got %d", got.TotalTokens)
	}
	if _, ok := got.APIs["imported-api"]; ok {
		t.Fatal("did not expect aggregate-only api data to merge into non-empty store")
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestSnapshotWithDetailLimit(t *testing.T) {
	stats := NewRequestStatistics()
	SetStatisticsEnabled(true)
	base := time.Date(2026, 2, 19, 11, 0, 0, 0, time.UTC)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: base,
		Detail: coreusage.Detail{
			InputTokens:     10,
			OutputTokens:    4,
			ReasoningTokens: 1,
			CachedTokens:    2,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-1",
		Model:       "model-1",
		RequestedAt: base.Add(1 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:     8,
			OutputTokens:    6,
			ReasoningTokens: 2,
			CachedTokens:    0,
		},
	})

	compact := stats.SnapshotWithDetailLimit(0)
	api, ok := compact.APIs["api-1"]
	if !ok {
		t.Fatal("expected api snapshot for api-1")
	}
	model, ok := api.Models["model-1"]
	if !ok {
		t.Fatal("expected model snapshot for model-1")
	}
	if len(model.Details) != 0 {
		t.Fatalf("expected details to be omitted, got %d", len(model.Details))
	}
	if model.InputTokens != 18 || model.OutputTokens != 10 || model.ReasoningTokens != 3 || model.CachedTokens != 2 {
		t.Fatalf(
			"unexpected model token breakdown: input=%d output=%d reasoning=%d cached=%d",
			model.InputTokens,
			model.OutputTokens,
			model.ReasoningTokens,
			model.CachedTokens,
		)
	}

	limited := stats.SnapshotWithDetailLimit(1)
	apiLimited := limited.APIs["api-1"]
	modelLimited := apiLimited.Models["model-1"]
	if len(modelLimited.Details) != 1 {
		t.Fatalf("expected only latest detail, got %d", len(modelLimited.Details))
	}
	if !modelLimited.Details[0].Timestamp.Equal(base.Add(1 * time.Minute)) {
		t.Fatalf("expected latest detail timestamp, got %s", modelLimited.Details[0].Timestamp)
	}

	full := stats.Snapshot()
	modelFull := full.APIs["api-1"].Models["model-1"]
	if len(modelFull.Details) != 2 {
		t.Fatalf("expected full snapshot details=2, got %d", len(modelFull.Details))
	}
}

func TestSnapshotRecentWithDetailLimit(t *testing.T) {
	stats := NewRequestStatistics()
	SetStatisticsEnabled(true)
	now := time.Now().UTC()

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-r",
		Model:       "model-r",
		RequestedAt: now.Add(-3 * time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  6,
			OutputTokens: 4,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "api-r",
		Model:       "model-r",
		RequestedAt: now.Add(-40 * time.Hour),
		Detail: coreusage.Detail{
			InputTokens:  3,
			OutputTokens: 2,
		},
	})

	snapshot := stats.SnapshotRecentWithDetailLimit(24, -1)
	if snapshot.TotalRequests != 1 {
		t.Fatalf("expected recent total_requests=1, got %d", snapshot.TotalRequests)
	}
	if snapshot.TotalTokens != 10 {
		t.Fatalf("expected recent total_tokens=10, got %d", snapshot.TotalTokens)
	}
	if snapshot.SuccessCount != 1 || snapshot.FailureCount != 0 {
		t.Fatalf("expected success/failure=1/0, got %d/%d", snapshot.SuccessCount, snapshot.FailureCount)
	}

	api := snapshot.APIs["api-r"]
	model := api.Models["model-r"]
	if model.TotalRequests != 1 || len(model.Details) != 1 {
		t.Fatalf("expected one recent model detail, got requests=%d details=%d", model.TotalRequests, len(model.Details))
	}

	compact := stats.SnapshotRecentWithDetailLimit(24, 0)
	modelCompact := compact.APIs["api-r"].Models["model-r"]
	if len(modelCompact.Details) != 0 {
		t.Fatalf("expected compact recent snapshot without details, got %d", len(modelCompact.Details))
	}
}
