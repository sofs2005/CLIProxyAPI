package auth

import (
	"testing"
	"time"
)

func TestCodexQuotaRetryAfterDuration_UsesPrimaryResetAt(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	resetAt := now.Add(90 * time.Minute)
	auth := &Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 100,
						"reset_at":     resetAt.Format(time.RFC3339),
					},
				},
			},
		},
	}

	got := codexQuotaRetryAfterDuration(auth, now)
	if got == nil {
		t.Fatal("expected retry after duration")
	}
	if *got != 90*time.Minute {
		t.Fatalf("retry after = %v, want 90m", *got)
	}
}

func TestCodexQuotaRetryAfterDuration_UnixResetAt(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	resetAt := now.Add(45 * time.Minute)
	auth := &Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 80,
						"reset_at":     float64(resetAt.Unix()),
					},
				},
			},
		},
	}

	got := codexQuotaRetryAfterDuration(auth, now)
	if got == nil {
		t.Fatal("expected retry after from future reset_at even when used_percent < 100")
	}
	if *got != 45*time.Minute {
		t.Fatalf("retry after = %v, want 45m", *got)
	}
}

func TestResolveQuotaCooldown_XAIDefault24h(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	auth := &Auth{Provider: "xai"}
	next, level := resolveQuotaCooldown(auth, nil, QuotaState{}, now)
	if level != 0 {
		t.Fatalf("backoff level = %d, want 0", level)
	}
	if !next.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("next = %v, want now+24h", next)
	}
}

func TestResolveQuotaCooldown_CodexMetadataFallback(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	resetAt := now.Add(3 * time.Hour)
	auth := &Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 100,
						"reset_at":     resetAt.Format(time.RFC3339),
					},
				},
			},
		},
	}
	next, _ := resolveQuotaCooldown(auth, nil, QuotaState{}, now)
	if !next.Equal(resetAt) {
		t.Fatalf("next = %v, want %v", next, resetAt)
	}
}

func TestResolveQuotaCooldown_ExplicitRetryAfterWins(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	wait := 12 * time.Second
	auth := &Auth{Provider: "xai"}
	next, _ := resolveQuotaCooldown(auth, &wait, QuotaState{}, now)
	if !next.Equal(now.Add(wait)) {
		t.Fatalf("next = %v, want now+12s", next)
	}
}
