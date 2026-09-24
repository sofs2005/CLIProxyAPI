package auth

import (
	"testing"
	"time"
)

func TestCodexPrimaryResetAt_ReportsFutureReset(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(2 * time.Hour)
	auth := &Auth{Provider: "codex", Metadata: codexResetMetadata(resetAt)}

	got, ok := CodexPrimaryResetAt(auth, now)
	if !ok {
		t.Fatal("CodexPrimaryResetAt() ok = false, want true")
	}
	if !got.Equal(resetAt.UTC()) {
		t.Fatalf("CodexPrimaryResetAt() = %s, want %s", got, resetAt.UTC())
	}
}

func TestCodexPrimaryResetAt_RejectsElapsedReset(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	auth := &Auth{Provider: "codex", Metadata: codexResetMetadata(now.Add(-time.Minute))}

	if _, ok := CodexPrimaryResetAt(auth, now); ok {
		t.Fatal("CodexPrimaryResetAt() ok = true for an elapsed reset, want false")
	}
}

func TestCodexPrimaryResetAt_RejectsMissingOrInvalidMetadata(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	cases := map[string]*Auth{
		"nil auth":         nil,
		"other provider":   {Provider: "claude", Metadata: codexResetMetadata(now.Add(time.Hour))},
		"no metadata":      {Provider: "codex"},
		"empty quota":      {Provider: "codex", Metadata: map[string]any{"codex_quota": map[string]any{}}},
		"unparsable reset": {Provider: "codex", Metadata: map[string]any{"codex_quota": map[string]any{"rate_limit": map[string]any{"primary_window": map[string]any{"reset_at": "not-a-time"}}}}},
	}

	for name, auth := range cases {
		if _, ok := CodexPrimaryResetAt(auth, now); ok {
			t.Fatalf("%s: CodexPrimaryResetAt() ok = true, want false", name)
		}
	}
}

func TestCodexPrimaryResetAt_AcceptsProviderCaseAndUnixSeconds(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(90 * time.Minute)
	auth := &Auth{
		Provider: "CODEX",
		Metadata: map[string]any{
			"codex_quota": map[string]any{
				"rate_limit": map[string]any{
					"primary_window": map[string]any{"reset_at": resetAt.Unix()},
				},
			},
		},
	}

	got, ok := CodexPrimaryResetAt(auth, now)
	if !ok {
		t.Fatal("CodexPrimaryResetAt() ok = false, want true")
	}
	if !got.Equal(time.Unix(resetAt.Unix(), 0).UTC()) {
		t.Fatalf("CodexPrimaryResetAt() = %s, want %s", got, time.Unix(resetAt.Unix(), 0).UTC())
	}
}
