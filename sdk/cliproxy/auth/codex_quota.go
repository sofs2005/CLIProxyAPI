package auth

import (
	"strconv"
	"strings"
	"time"
)

const (
	codexQuotaMetadataKey      = "codex_quota"
	codexQuotaExhaustedPercent = 100
)

// codexQuotaPrimaryResetAt returns the earliest future primary-window reset time
// recorded in auth metadata, when present.
func codexQuotaPrimaryResetAt(auth *Auth, now time.Time) (time.Time, bool) {
	window := codexQuotaPrimaryWindow(auth)
	if window == nil {
		return time.Time{}, false
	}
	resetAt, ok := parseCodexQuotaResetAt(window["reset_at"])
	if !ok || !resetAt.After(now) {
		return time.Time{}, false
	}
	return resetAt, true
}

// codexQuotaMetadataBlocking reports whether codex_quota metadata indicates the
// account's primary usage window is exhausted until reset_at.
func codexQuotaMetadataBlocking(auth *Auth, now time.Time) (bool, time.Time) {
	if auth == nil || !strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") {
		return false, time.Time{}
	}
	window := codexQuotaPrimaryWindow(auth)
	if window == nil {
		return false, time.Time{}
	}
	used, okUsed := parseCodexQuotaUsedPercent(window["used_percent"])
	if !okUsed || used < codexQuotaExhaustedPercent {
		return false, time.Time{}
	}
	resetAt, okReset := parseCodexQuotaResetAt(window["reset_at"])
	if !okReset || !resetAt.After(now) {
		return false, time.Time{}
	}
	return true, resetAt
}

// codexQuotaRetryAfterDuration returns a RetryAfter-style duration derived from
// codex_quota metadata when the primary window is exhausted.
func codexQuotaRetryAfterDuration(auth *Auth, now time.Time) *time.Duration {
	blocked, resetAt := codexQuotaMetadataBlocking(auth, now)
	if !blocked {
		// Even without used_percent==100, a future reset_at is still the best
		// recovery hint when the provider omitted resets_at on a 429.
		if resetAt, ok := codexQuotaPrimaryResetAt(auth, now); ok {
			wait := resetAt.Sub(now)
			if wait > 0 {
				return &wait
			}
		}
		return nil
	}
	wait := resetAt.Sub(now)
	if wait <= 0 {
		return nil
	}
	return &wait
}

func codexQuotaPrimaryWindow(auth *Auth) map[string]any {
	if auth == nil || auth.Metadata == nil {
		return nil
	}
	quota, ok := auth.Metadata[codexQuotaMetadataKey].(map[string]any)
	if !ok || quota == nil {
		return nil
	}
	rateLimit, ok := quota["rate_limit"].(map[string]any)
	if !ok || rateLimit == nil {
		return nil
	}
	primary, ok := rateLimit["primary_window"].(map[string]any)
	if !ok || primary == nil {
		return nil
	}
	return primary
}

func parseCodexQuotaUsedPercent(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func parseCodexQuotaResetAt(raw any) (time.Time, bool) {
	switch value := raw.(type) {
	case time.Time:
		if value.IsZero() {
			return time.Time{}, false
		}
		return value.UTC(), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return time.Time{}, false
		}
		if unix, err := strconv.ParseInt(trimmed, 10, 64); err == nil && unix > 0 {
			return time.Unix(unix, 0).UTC(), true
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.UTC(), true
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC(), true
		}
		return time.Time{}, false
	case float64:
		if value <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(value), 0).UTC(), true
	case int64:
		if value <= 0 {
			return time.Time{}, false
		}
		return time.Unix(value, 0).UTC(), true
	case int:
		if value <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(value), 0).UTC(), true
	default:
		return time.Time{}, false
	}
}
