package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type refreshOnlyExecutor struct {
	id string
}

func (e *refreshOnlyExecutor) Identifier() string { return e.id }

func (e *refreshOnlyExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusNotImplemented, Message: "not implemented"}
}

func (e *refreshOnlyExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, &Error{HTTPStatus: http.StatusNotImplemented, Message: "not implemented"}
}

func (e *refreshOnlyExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	auth.Metadata["access_token"] = "refreshed-access-token"
	return auth, nil
}

func (e *refreshOnlyExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusNotImplemented, Message: "not implemented"}
}

func (e *refreshOnlyExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestRefreshAuthForRequest_PreservesQuotaErrorState(t *testing.T) {
	t.Parallel()

	model := "grok-4.5"
	next := time.Now().Add(24 * time.Hour)
	auth := &Auth{
		ID:             "xai-quota-auth",
		Provider:       "xai",
		Status:         StatusError,
		StatusMessage:  "quota exhausted",
		Unavailable:    true,
		NextRetryAfter: next,
		LastError: &Error{
			HTTPStatus: http.StatusTooManyRequests,
			Message:    "free-usage-exhausted",
			Code:       "quota",
		},
		Quota: QuotaState{
			Exceeded:      true,
			Reason:        "quota",
			NextRecoverAt: next,
			BackoffLevel:  1,
		},
		Metadata: map[string]any{
			"access_token":  "stale-access-token",
			"refresh_token": "refresh-token",
		},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				StatusMessage:  "quota exhausted",
				Unavailable:    true,
				NextRetryAfter: next,
				LastError: &Error{
					HTTPStatus: http.StatusTooManyRequests,
					Message:    "free-usage-exhausted",
					Code:       "quota",
				},
				Quota: QuotaState{
					Exceeded:      true,
					Reason:        "quota",
					NextRecoverAt: next,
					BackoffLevel:  1,
				},
			},
		},
	}

	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(&refreshOnlyExecutor{id: "xai"})
	if _, err := m.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	updated, err := m.refreshAuthForRequest(context.Background(), auth.ID, "")
	if err != nil {
		t.Fatalf("refreshAuthForRequest error = %v", err)
	}
	if updated == nil {
		t.Fatal("refreshAuthForRequest returned nil auth")
	}
	if got := authAccessToken(updated); got != "refreshed-access-token" {
		t.Fatalf("access_token = %q, want refreshed-access-token", got)
	}
	if updated.Status != StatusError {
		t.Fatalf("Status = %q, want %q (quota failure must survive token refresh)", updated.Status, StatusError)
	}
	if updated.StatusMessage != "quota exhausted" {
		t.Fatalf("StatusMessage = %q, want quota exhausted", updated.StatusMessage)
	}
	if updated.LastError == nil || updated.LastError.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("LastError = %#v, want 429 quota error", updated.LastError)
	}
	if !updated.Quota.Exceeded {
		t.Fatalf("Quota.Exceeded = false, want true")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatal("model state missing after refresh")
	}
	if state.Status != StatusError || !state.Unavailable {
		t.Fatalf("model state status=%q unavailable=%v, want error/true", state.Status, state.Unavailable)
	}
	if state.LastError == nil || state.LastError.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("model LastError = %#v, want 429", state.LastError)
	}
	if state.NextRetryAfter.IsZero() || state.NextRetryAfter.Before(time.Now()) {
		t.Fatalf("model NextRetryAfter = %v, want future cooldown", state.NextRetryAfter)
	}
}

func TestRefreshAuthForRequest_ClearsUnauthorizedState(t *testing.T) {
	t.Parallel()

	model := "grok-4.5"
	next := time.Now().Add(30 * time.Minute)
	auth := &Auth{
		ID:             "xai-unauth",
		Provider:       "xai",
		Status:         StatusError,
		StatusMessage:  "unauthorized",
		Unavailable:    true,
		NextRetryAfter: next,
		LastError: &Error{
			HTTPStatus: http.StatusUnauthorized,
			Message:    "token invalidated",
			Code:       "unauthorized",
		},
		Metadata: map[string]any{
			"access_token":  "stale-access-token",
			"refresh_token": "refresh-token",
		},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				StatusMessage:  "unauthorized",
				Unavailable:    true,
				NextRetryAfter: next,
				LastError: &Error{
					HTTPStatus: http.StatusUnauthorized,
					Message:    "token invalidated",
					Code:       "unauthorized",
				},
			},
		},
	}

	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(&refreshOnlyExecutor{id: "xai"})
	if _, err := m.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	updated, err := m.refreshAuthForRequest(context.Background(), auth.ID, "")
	if err != nil {
		t.Fatalf("refreshAuthForRequest error = %v", err)
	}
	if updated == nil {
		t.Fatal("refreshAuthForRequest returned nil auth")
	}
	if updated.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusActive)
	}
	if updated.StatusMessage != "" {
		t.Fatalf("StatusMessage = %q, want empty", updated.StatusMessage)
	}
	if updated.LastError != nil {
		t.Fatalf("LastError = %#v, want nil", updated.LastError)
	}
	if updated.Unavailable {
		t.Fatalf("Unavailable = true, want false")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatal("model state missing after refresh")
	}
	if state.Status != StatusActive || state.Unavailable || state.LastError != nil {
		t.Fatalf("model state = status %q unavailable %v lastError %#v, want cleared", state.Status, state.Unavailable, state.LastError)
	}
}

func TestRefreshAuthForRequest_ClearsForbiddenState(t *testing.T) {
	t.Parallel()

	auth := &Auth{
		ID:            "xai-forbidden",
		Provider:      "xai",
		Status:        StatusError,
		StatusMessage: "payment_required",
		Unavailable:   true,
		LastError: &Error{
			HTTPStatus: http.StatusForbidden,
			Message:    "forbidden",
		},
		Metadata: map[string]any{
			"access_token":  "stale-access-token",
			"refresh_token": "refresh-token",
		},
	}

	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(&refreshOnlyExecutor{id: "xai"})
	if _, err := m.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	updated, err := m.refreshAuthForRequest(context.Background(), auth.ID, "")
	if err != nil {
		t.Fatalf("refreshAuthForRequest error = %v", err)
	}
	if updated.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusActive)
	}
	if updated.LastError != nil || updated.StatusMessage != "" || updated.Unavailable {
		t.Fatalf("credential failure not cleared: statusMessage=%q lastError=%#v unavailable=%v",
			updated.StatusMessage, updated.LastError, updated.Unavailable)
	}
}

func TestIsCredentialAuthFailureError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  *Error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "401", err: &Error{HTTPStatus: http.StatusUnauthorized}, want: true},
		{name: "403", err: &Error{HTTPStatus: http.StatusForbidden}, want: true},
		{name: "unauthorized code", err: &Error{Code: "unauthorized"}, want: true},
		{name: "429", err: &Error{HTTPStatus: http.StatusTooManyRequests, Code: "quota"}, want: false},
		{name: "500", err: &Error{HTTPStatus: http.StatusInternalServerError}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCredentialAuthFailureError(tc.err); got != tc.want {
				t.Fatalf("isCredentialAuthFailureError() = %v, want %v", got, tc.want)
			}
		})
	}
}
