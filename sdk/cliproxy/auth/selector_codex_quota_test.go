package auth

import (
	"testing"
	"time"
)

func TestIsAuthBlockedForModel_CodexQuotaMetadataBlocksWhenExhausted(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	resetAt := now.Add(2 * time.Hour)
	auth := &Auth{
		ID:       "codex-1",
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

	blocked, reason, next := isAuthBlockedForModel(auth, "gpt-5-codex", now)
	if !blocked {
		t.Fatal("expected exhausted codex_quota metadata to block selection")
	}
	if reason != blockReasonCooldown {
		t.Fatalf("reason = %v, want %v", reason, blockReasonCooldown)
	}
	if next.IsZero() || next.Before(now) {
		t.Fatalf("next = %v, want future reset", next)
	}
}

func TestIsAuthBlockedForModel_CodexQuotaMetadataBelow100DoesNotBlock(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	auth := &Auth{
		ID:       "codex-1",
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 99,
						"reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}

	blocked, reason, _ := isAuthBlockedForModel(auth, "gpt-5-codex", now)
	if blocked {
		t.Fatalf("expected non-exhausted codex quota not to block selection, got reason %v", reason)
	}
}

func TestIsAuthBlockedForModel_CodexQuotaMetadataWithExpiredResetDoesNotBlockSelection(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	auth := &Auth{
		ID:       "codex-1",
		Provider: "codex",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"used_percent": 100,
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

func TestIsAuthBlockedForModel_CodexQuotaMetadataDoesNotAffectOtherProviders(t *testing.T) {
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

func TestIsAuthBlockedForModel_AuthLevelQuotaBlocksOtherModels(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	next := now.Add(24 * time.Hour)
	auth := &Auth{
		ID:             "xai-1",
		Provider:       "xai",
		Quota:          QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next},
		NextRetryAfter: next,
		Unavailable:    true,
		ModelStates: map[string]*ModelState{
			"grok-4.5": {
				Status:         StatusError,
				Unavailable:    true,
				NextRetryAfter: next,
				Quota:          QuotaState{Exceeded: true, NextRecoverAt: next},
			},
		},
	}

	blocked, reason, gotNext := isAuthBlockedForModel(auth, "grok-3", now)
	if !blocked {
		t.Fatal("expected account-level quota to block other models")
	}
	if reason != blockReasonCooldown {
		t.Fatalf("reason = %v, want %v", reason, blockReasonCooldown)
	}
	if !gotNext.Equal(next) {
		t.Fatalf("next = %v, want %v", gotNext, next)
	}
}
