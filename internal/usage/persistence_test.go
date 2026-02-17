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
