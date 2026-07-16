package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func TestReconcileRegistryModelStates_PreservesActiveQuotaCooldown(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	ctx := context.Background()
	authID := "xai-reconcile-preserve"
	model := "grok-4.5"
	next := time.Now().Add(24 * time.Hour)

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(authID, "xai", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() {
		reg.UnregisterClient(authID)
	})

	if _, err := manager.Register(ctx, &Auth{
		ID:             authID,
		Provider:       "xai",
		Status:         StatusError,
		StatusMessage:  "free-usage-exhausted",
		Unavailable:    true,
		NextRetryAfter: next,
		LastError: &Error{
			HTTPStatus: http.StatusTooManyRequests,
			Message:    "free-usage-exhausted",
			Code:       "quota",
		},
		Quota: QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next, BackoffLevel: 1},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				StatusMessage:  "free-usage-exhausted",
				Unavailable:    true,
				NextRetryAfter: next,
				LastError: &Error{
					HTTPStatus: http.StatusTooManyRequests,
					Message:    "free-usage-exhausted",
					Code:       "quota",
				},
				Quota: QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next, BackoffLevel: 1},
			},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	manager.ReconcileRegistryModelStates(ctx, authID)

	updated, ok := manager.GetByID(authID)
	if !ok || updated == nil {
		t.Fatal("auth missing after reconcile")
	}
	if updated.Status != StatusError {
		t.Fatalf("Status = %q, want error after reconcile", updated.Status)
	}
	if !updated.Quota.Exceeded {
		t.Fatal("Quota.Exceeded wiped by reconcile")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatal("model state missing")
	}
	if state.Status != StatusError || !state.Unavailable {
		t.Fatalf("model state cleared: status=%q unavailable=%v", state.Status, state.Unavailable)
	}
	if state.NextRetryAfter.IsZero() || !state.NextRetryAfter.After(time.Now()) {
		t.Fatalf("model NextRetryAfter wiped: %v", state.NextRetryAfter)
	}
	if !state.Quota.Exceeded {
		t.Fatal("model Quota.Exceeded wiped by reconcile")
	}
}

func TestReconcileRegistryModelStates_ResetsExpiredCooldown(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	ctx := context.Background()
	authID := "xai-reconcile-expire"
	model := "grok-4.5"
	past := time.Now().Add(-time.Hour)

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(authID, "xai", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() {
		reg.UnregisterClient(authID)
	})

	if _, err := manager.Register(ctx, &Auth{
		ID:             authID,
		Provider:       "xai",
		Status:         StatusError,
		StatusMessage:  "quota exhausted",
		Unavailable:    true,
		NextRetryAfter: past,
		Quota:          QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: past},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				StatusMessage:  "quota exhausted",
				Unavailable:    true,
				NextRetryAfter: past,
				Quota:          QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: past},
			},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	manager.ReconcileRegistryModelStates(ctx, authID)

	updated, ok := manager.GetByID(authID)
	if !ok || updated == nil {
		t.Fatal("auth missing after reconcile")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatal("model state missing")
	}
	if state.Status != StatusActive || state.Unavailable || state.Quota.Exceeded {
		t.Fatalf("expired cooldown not reset: status=%q unavailable=%v quota=%+v", state.Status, state.Unavailable, state.Quota)
	}
}

func TestManager_Update_PreservesQuotaCooldownState(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	ctx := context.Background()
	authID := "xai-update-preserve"
	model := "grok-4.5"
	next := time.Now().Add(24 * time.Hour)

	if _, err := manager.Register(ctx, &Auth{
		ID:             authID,
		Provider:       "xai",
		Status:         StatusError,
		StatusMessage:  "free-usage-exhausted",
		Unavailable:    true,
		NextRetryAfter: next,
		LastError: &Error{
			HTTPStatus: http.StatusTooManyRequests,
			Message:    "free-usage-exhausted",
			Code:       "quota",
		},
		Quota: QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next, BackoffLevel: 1},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				Unavailable:    true,
				NextRetryAfter: next,
				Quota:          QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next},
			},
		},
		Metadata: map[string]any{"access_token": "tok"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// File synthesizer reload: StatusActive, empty error/quota, empty ModelStates.
	if _, err := manager.Update(ctx, &Auth{
		ID:       authID,
		Provider: "xai",
		Status:   StatusActive,
		Metadata: map[string]any{"access_token": "tok2"},
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, ok := manager.GetByID(authID)
	if !ok || updated == nil {
		t.Fatal("auth missing after update")
	}
	if updated.Status != StatusError {
		t.Fatalf("Status = %q, want error preserved across file reload", updated.Status)
	}
	if !updated.Quota.Exceeded {
		t.Fatal("Quota.Exceeded wiped by file Update")
	}
	if updated.LastError == nil || updated.LastError.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("LastError wiped: %#v", updated.LastError)
	}
	if !updated.Unavailable {
		t.Fatal("Unavailable wiped by file Update")
	}
	if updated.NextRetryAfter.IsZero() || !updated.NextRetryAfter.After(time.Now()) {
		t.Fatalf("NextRetryAfter wiped: %v", updated.NextRetryAfter)
	}
	if len(updated.ModelStates) == 0 {
		t.Fatal("ModelStates wiped by file Update")
	}
}

func TestManager_MarkResult_XAI429BlocksSelection(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	ctx := context.Background()
	authID := "xai-mark-429"
	model := "grok-4.5"

	if _, err := manager.Register(ctx, &Auth{
		ID:       authID,
		Provider: "xai",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	retryAfter := 24 * time.Hour
	manager.MarkResult(ctx, Result{
		AuthID:     authID,
		Provider:   "xai",
		Model:      model,
		Success:    false,
		RetryAfter: &retryAfter,
		Error: &Error{
			HTTPStatus: http.StatusTooManyRequests,
			Message:    "free-usage-exhausted",
			Code:       "quota",
		},
	})

	updated, ok := manager.GetByID(authID)
	if !ok || updated == nil {
		t.Fatal("auth missing")
	}
	if !updated.Quota.Exceeded {
		t.Fatal("expected auth Quota.Exceeded after 429")
	}

	now := time.Now()
	// Same model
	blocked, reason, next := isAuthBlockedForModel(updated, model, now)
	if !blocked {
		t.Fatal("expected 429 account blocked for same model")
	}
	if reason != blockReasonCooldown {
		t.Fatalf("reason = %v, want cooldown", reason)
	}
	if next.IsZero() || !next.After(now) {
		t.Fatalf("next = %v, want future", next)
	}
	// Different model alias — account-scoped blackout
	blocked, reason, _ = isAuthBlockedForModel(updated, "grok-3", now)
	if !blocked {
		t.Fatal("expected 429 account blocked for other model")
	}
	if reason != blockReasonCooldown {
		t.Fatalf("reason = %v, want cooldown", reason)
	}
}
