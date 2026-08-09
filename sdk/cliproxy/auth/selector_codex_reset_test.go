package auth

import (
	"context"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func newCodexAuthWithReset(id string, resetAt *time.Time) *Auth {
	auth := &Auth{ID: id, Provider: "codex"}
	if resetAt == nil {
		return auth
	}
	auth.Metadata = map[string]any{
		"codex_quota": map[string]any{
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 50,
					"reset_at":     resetAt.Format(time.RFC3339Nano),
				},
			},
		},
	}
	return auth
}

func authIDs(auths []*Auth) []string {
	ids := make([]string, 0, len(auths))
	for _, auth := range auths {
		if auth == nil {
			ids = append(ids, "")
			continue
		}
		ids = append(ids, auth.ID)
	}
	return ids
}

func TestPrioritizeCodexExpiringAuths_OrdersKnownResetsBeforeUnknown(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	farReset := now.Add(3 * time.Hour)
	middleReset := now.Add(2 * time.Hour)
	nearReset := now.Add(time.Hour)
	auths := []*Auth{
		newCodexAuthWithReset("far", &farReset),
		newCodexAuthWithReset("unknown-a", nil),
		newCodexAuthWithReset("middle", &middleReset),
		newCodexAuthWithReset("near", &nearReset),
		newCodexAuthWithReset("unknown-b", nil),
	}

	got := prioritizeCodexExpiringAuths("codex", auths, now)
	want := []string{"near", "middle", "far", "unknown-a", "unknown-b"}
	if gotIDs := authIDs(got); !equalStringSlices(gotIDs, want) {
		t.Fatalf("ordered auth IDs = %v, want %v", gotIDs, want)
	}
}

func TestPrioritizeCodexExpiringAuths_NonCodexAndExpiredResetsPreserveOrder(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	expiredReset := now.Add(-time.Minute)
	auths := []*Auth{
		newCodexAuthWithReset("expired", &expiredReset),
		newCodexAuthWithReset("unknown", nil),
	}
	original := authIDs(auths)

	if got := prioritizeCodexExpiringAuths("gemini", auths, now); !equalStringSlices(authIDs(got), original) {
		t.Fatalf("non-Codex order = %v, want %v", authIDs(got), original)
	}
	if got := prioritizeCodexExpiringAuths("codex", auths, now); !equalStringSlices(authIDs(got), original) {
		t.Fatalf("expired-reset order = %v, want %v", authIDs(got), original)
	}
	if got := prioritizeCodexExpiringAuths("codex", nil, now); got != nil {
		t.Fatalf("nil input = %v, want nil", got)
	}
}

func TestLessFillFirstAuth_CodexResetPriorityKeepsDemotionDominant(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	nearReset := now.Add(time.Hour)
	farReset := now.Add(2 * time.Hour)
	near := newCodexAuthWithReset("near", &nearReset)
	far := newCodexAuthWithReset("far", &farReset)

	if !lessFillFirstAuth(near, far, "model", 42, now) {
		t.Fatal("expected nearer Codex reset to rank first")
	}

	near.ModelStates = map[string]*ModelState{
		"model": {FillFirstDemoted: true},
	}
	if lessFillFirstAuth(near, far, "model", 42, now) {
		t.Fatal("expected non-demoted Codex auth to rank before nearer reset")
	}

	unknown := newCodexAuthWithReset("unknown", nil)
	if !lessFillFirstAuth(far, unknown, "model", 42, now) {
		t.Fatal("expected known Codex reset to rank before unknown reset")
	}
}

func TestLessFillFirstAuth_CodexEqualResetFallsBackToShuffleRank(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	resetAt := now.Add(time.Hour)
	left := newCodexAuthWithReset("left", &resetAt)
	right := newCodexAuthWithReset("right", &resetAt)
	const seed uint64 = 42

	leftRank := fillFirstShuffleRank(seed, left.ID)
	rightRank := fillFirstShuffleRank(seed, right.ID)
	wantLeft := leftRank < rightRank || (leftRank == rightRank && left.ID < right.ID)
	if got := lessFillFirstAuth(left, right, "model", seed, now); got != wantLeft {
		t.Fatalf("left-before-right = %v, want %v", got, wantLeft)
	}
}

func TestRoundRobinSelectorPick_CodexNearestResetFirst(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	farReset := now.Add(3 * time.Hour)
	middleReset := now.Add(2 * time.Hour)
	nearReset := now.Add(time.Hour)
	selector := &RoundRobinSelector{}
	auths := []*Auth{
		newCodexAuthWithReset("far", &farReset),
		newCodexAuthWithReset("middle", &middleReset),
		newCodexAuthWithReset("near", &nearReset),
	}
	want := []string{"near", "middle", "far"}
	for index, wantID := range want {
		got, errPick := selector.Pick(context.Background(), "codex", "gpt-5-codex", cliproxyexecutor.Options{}, auths)
		if errPick != nil {
			t.Fatalf("Pick() #%d error = %v", index, errPick)
		}
		if got == nil || got.ID != wantID {
			t.Fatalf("Pick() #%d auth = %#v, want %s", index, got, wantID)
		}
	}
}

func TestWeightedRoundRobinSelectorPick_CodexNearestResetFirst(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	farReset := now.Add(3 * time.Hour)
	nearReset := now.Add(time.Hour)
	selector := &WeightedRoundRobinSelector{}
	auths := []*Auth{
		newCodexAuthWithReset("far", &farReset),
		newCodexAuthWithReset("near", &nearReset),
	}

	got, errPick := selector.Pick(context.Background(), "codex", "gpt-5-codex", cliproxyexecutor.Options{}, auths)
	if errPick != nil {
		t.Fatalf("Pick() error = %v", errPick)
	}
	if got == nil || got.ID != "near" {
		t.Fatalf("Pick() auth = %#v, want near", got)
	}
}

func TestFillFirstSelectorPick_CodexNearestResetFirst(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	farReset := now.Add(3 * time.Hour)
	nearReset := now.Add(time.Hour)
	selector := &FillFirstSelector{seed: 42}
	auths := []*Auth{
		newCodexAuthWithReset("far", &farReset),
		newCodexAuthWithReset("near", &nearReset),
	}

	got, errPick := selector.Pick(context.Background(), "codex", "gpt-5-codex", cliproxyexecutor.Options{}, auths)
	if errPick != nil {
		t.Fatalf("Pick() error = %v", errPick)
	}
	if got == nil || got.ID != "near" {
		t.Fatalf("Pick() auth = %#v, want near", got)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
