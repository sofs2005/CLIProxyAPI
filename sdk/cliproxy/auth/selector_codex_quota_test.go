package auth

import (
	"testing"
	"time"
)

func TestIsAuthBlockedForModel_CodexQuotaReserveBlocksNearLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	auth := &Auth{
		ID:       "codex-1",
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 90,
						"reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}

	blocked, reason, next := isAuthBlockedForModel(auth, "gpt-5-codex", now)
	if !blocked {
		t.Fatal("expected codex auth to be blocked when used_percent >= 90")
	}
	if reason != blockReasonCooldown {
		t.Fatalf("block reason = %v, want cooldown", reason)
	}
	if !next.After(now) {
		t.Fatalf("next reset time = %v, want future time", next)
	}
}

func TestIsAuthBlockedForModel_CodexQuotaReserveExpiresAfterReset(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	auth := &Auth{
		ID:       "codex-1",
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 95,
						"reset_at":     now.Add(-1 * time.Minute).Format(time.RFC3339),
					},
				},
			},
		},
	}

	blocked, _, _ := isAuthBlockedForModel(auth, "gpt-5-codex", now)
	if blocked {
		t.Fatal("expected codex auth to become available after reset_at has passed")
	}
}

func TestIsAuthBlockedForModel_CodexQuotaReserveDoesNotAffectOtherProviders(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	auth := &Auth{
		ID:       "claude-1",
		Provider: "claude",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 100,
						"reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}

	blocked, _, _ := isAuthBlockedForModel(auth, "claude-sonnet-4", now)
	if blocked {
		t.Fatal("expected non-codex providers to ignore codex quota metadata")
	}
}
