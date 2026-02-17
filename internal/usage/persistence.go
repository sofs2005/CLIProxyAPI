package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

const defaultPersistenceInterval = 30 * time.Minute

type persistencePayload struct {
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exported_at"`
	Usage      StatisticsSnapshot `json:"usage"`
}

var (
	persistenceMu     sync.Mutex
	persistencePath   string
	persistenceCancel context.CancelFunc
	persistenceDone   chan struct{}
	persistenceDirty  atomic.Bool
)

// StartPersistence enables periodic usage statistics persistence to a JSON file.
// Existing persisted data will be loaded and merged into the in-memory store.
func StartPersistence(path string, interval time.Duration) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("usage persistence: path is empty")
	}
	if interval <= 0 {
		interval = defaultPersistenceInterval
	}
	if absPath, errAbs := filepath.Abs(path); errAbs == nil {
		path = absPath
	}

	_ = StopPersistence(context.Background())

	if errLoad := loadPersistence(path); errLoad != nil {
		return errLoad
	}
	// Loaded data should not immediately trigger a rewrite.
	persistenceDirty.Store(false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	persistenceMu.Lock()
	persistencePath = path
	persistenceCancel = cancel
	persistenceDone = done
	persistenceMu.Unlock()

	go runPersistenceLoop(ctx, path, interval, done)
	return nil
}

// StopPersistence flushes pending usage statistics and stops the background saver.
func StopPersistence(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	persistenceMu.Lock()
	cancel := persistenceCancel
	done := persistenceDone
	path := persistencePath
	persistenceCancel = nil
	persistenceDone = nil
	persistencePath = ""
	persistenceMu.Unlock()

	if cancel == nil {
		return nil
	}

	cancel()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if errFlush := flushPersistence(path, true); errFlush != nil {
		return errFlush
	}
	return nil
}

func runPersistenceLoop(ctx context.Context, path string, interval time.Duration, done chan<- struct{}) {
	defer close(done)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if errFlush := flushPersistence(path, false); errFlush != nil {
				log.WithError(errFlush).Warn("usage persistence tick flush failed")
			}
		case <-ctx.Done():
			if errFlush := flushPersistence(path, true); errFlush != nil {
				log.WithError(errFlush).Warn("usage persistence shutdown flush failed")
			}
			return
		}
	}
}

func flushPersistence(path string, force bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if !force && !persistenceDirty.Load() {
		return nil
	}

	snapshot := GetRequestStatistics().Snapshot()
	if errPersist := persistSnapshot(path, snapshot); errPersist != nil {
		return errPersist
	}
	persistenceDirty.Store(false)
	return nil
}

func loadPersistence(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("usage persistence: read snapshot failed: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	snapshot, errDecode := decodeSnapshot(data)
	if errDecode != nil {
		return fmt.Errorf("usage persistence: decode snapshot failed: %w", errDecode)
	}

	result := GetRequestStatistics().MergeSnapshot(snapshot)
	if result.Added > 0 {
		log.Infof("usage persistence loaded: added=%d skipped=%d", result.Added, result.Skipped)
	}
	return nil
}

func decodeSnapshot(data []byte) (StatisticsSnapshot, error) {
	var payload persistencePayload
	if err := json.Unmarshal(data, &payload); err == nil {
		if payload.Version != 0 && payload.Version != 1 {
			return StatisticsSnapshot{}, fmt.Errorf("unsupported usage snapshot version: %d", payload.Version)
		}
		if payload.Version != 0 || !payload.ExportedAt.IsZero() || hasSnapshotData(payload.Usage) {
			return payload.Usage, nil
		}
	}

	var snapshot StatisticsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return StatisticsSnapshot{}, err
	}
	return snapshot, nil
}

func hasSnapshotData(snapshot StatisticsSnapshot) bool {
	if snapshot.TotalRequests != 0 || snapshot.SuccessCount != 0 || snapshot.FailureCount != 0 || snapshot.TotalTokens != 0 {
		return true
	}
	return len(snapshot.APIs) > 0 || len(snapshot.RequestsByDay) > 0 || len(snapshot.RequestsByHour) > 0 || len(snapshot.TokensByDay) > 0 || len(snapshot.TokensByHour) > 0
}

func persistSnapshot(path string, snapshot StatisticsSnapshot) error {
	payload := persistencePayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("usage persistence: marshal snapshot failed: %w", err)
	}

	if errMkdir := os.MkdirAll(filepath.Dir(path), 0o755); errMkdir != nil {
		return fmt.Errorf("usage persistence: create directory failed: %w", errMkdir)
	}
	if errWrite := os.WriteFile(path, data, 0o600); errWrite != nil {
		return fmt.Errorf("usage persistence: write snapshot failed: %w", errWrite)
	}
	return nil
}
