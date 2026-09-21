package auth

import (
	"context"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// These tests exercise the scheduler fast path reached through Manager.pickNext,
// which is the path the default routing configuration actually uses. The
// selector-level tests in selector_codex_reset_test.go cover Selector.Pick only,
// and therefore cannot catch a regression in the scheduler's own ordering.
//
// The scheduler reads auth/model support from the process-global registry, so every
// test here must use its own model name and auth IDs to stay isolated under t.Parallel.

// newFillFirstResetManager builds a Manager wired for fill-first routing and returns it
// alongside the fixed shuffle seed shared with its selector.
func newFillFirstResetManager(t *testing.T) (*Manager, uint64) {
	t.Helper()

	const seed uint64 = 42
	selector := &FillFirstSelector{seed: seed}
	manager := NewManager(nil, selector, nil)
	return manager, seed
}

// registerResetAuths registers auths with the manager and the model registry so the
// scheduler builds a ready shard for model.
func registerResetAuths(t *testing.T, manager *Manager, provider, model string, auths ...*Auth) {
	t.Helper()

	ids := make([]string, 0, len(auths))
	for _, auth := range auths {
		auth.Provider = provider
		if _, errRegister := manager.Register(context.Background(), auth); errRegister != nil {
			t.Fatalf("Register(%s) error = %v", auth.ID, errRegister)
		}
		ids = append(ids, auth.ID)
	}
	registerSchedulerModels(t, provider, model, ids...)
	for _, auth := range auths {
		manager.RefreshSchedulerEntry(auth.ID)
	}
}

// codexResetMetadata builds the codex_quota metadata shape read by codexQuotaPrimaryResetAt,
// shared with selector_codex_reset_test.go's newCodexAuthWithReset.
func codexResetMetadata(resetAt time.Time) map[string]any {
	return map[string]any{
		"codex_quota": map[string]any{
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 50,
					"reset_at":     resetAt.Format(time.RFC3339Nano),
				},
			},
		},
	}
}

// codexResetCodexAuth builds a Codex credential carrying reset_at, using the shared
// metadata shape from selector_codex_reset_test.go.
func codexResetCodexAuth(prefix, suffix string, resetAt *time.Time) *Auth {
	auth := newCodexAuthWithReset(prefix+"-"+suffix, resetAt)
	auth.Provider = "codex"
	return auth
}

// pickNextIDs drives Manager.pickNext repeatedly, excluding each pick from later ones.
func pickNextIDs(t *testing.T, manager *Manager, provider, model string, count int) []string {
	t.Helper()

	tried := make(map[string]struct{}, count)
	picked := make([]string, 0, count)
	for index := 0; index < count; index++ {
		auth, _, errPick := manager.pickNext(context.Background(), provider, model, cliproxyexecutor.Options{}, tried)
		if errPick != nil {
			t.Fatalf("pickNext() #%d error = %v", index, errPick)
		}
		if auth == nil {
			t.Fatalf("pickNext() #%d auth = nil", index)
		}
		picked = append(picked, auth.ID)
		tried[auth.ID] = struct{}{}
	}
	return picked
}

func assertPickOrder(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("pick order = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("pick order = %v, want %v", got, want)
		}
	}
}

func TestManagerFillFirst_CodexResetOrdersProductionPath(t *testing.T) {
	t.Parallel()

	const model = "codex-reset-production-model"
	manager, _ := newFillFirstResetManager(t)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	now := time.Now()
	nearReset := now.Add(time.Hour)
	middleReset := now.Add(2 * time.Hour)
	farReset := now.Add(3 * time.Hour)

	// Registration order is deliberately the reverse of the expected pick order so a
	// passing result cannot come from insertion order.
	registerResetAuths(t, manager, "codex", model,
		codexResetCodexAuth("prod", "far", &farReset),
		codexResetCodexAuth("prod", "middle", &middleReset),
		codexResetCodexAuth("prod", "near", &nearReset),
	)

	assertPickOrder(t, pickNextIDs(t, manager, "codex", model, 3),
		[]string{"prod-near", "prod-middle", "prod-far"})
}

func TestManagerFillFirst_CodexStaleResetRanksAfterLiveReset(t *testing.T) {
	t.Parallel()

	const model = "codex-reset-stale-model"
	manager, _ := newFillFirstResetManager(t)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	now := time.Now()
	liveReset := now.Add(2 * time.Hour)
	staleReset := now.Add(-2 * time.Hour)

	registerResetAuths(t, manager, "codex", model,
		codexResetCodexAuth("stale", "expired", &staleReset),
		codexResetCodexAuth("stale", "live", &liveReset),
	)

	// An already-expired reset_at is treated as unknown, so it must not outrank a live one.
	assertPickOrder(t, pickNextIDs(t, manager, "codex", model, 1), []string{"stale-live"})
}

func TestManagerFillFirst_CodexDemotionOutranksReset(t *testing.T) {
	t.Parallel()

	const model = "codex-reset-demotion-model"
	manager, _ := newFillFirstResetManager(t)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	now := time.Now()
	nearReset := now.Add(time.Hour)
	middleReset := now.Add(2 * time.Hour)

	near := codexResetCodexAuth("demote", "near", &nearReset)
	near.ModelStates = map[string]*ModelState{model: {FillFirstDemoted: true}}

	registerResetAuths(t, manager, "codex", model,
		near,
		codexResetCodexAuth("demote", "middle", &middleReset),
	)

	// Demotion dominates, matching lessFillFirstAuth on the legacy selector path.
	assertPickOrder(t, pickNextIDs(t, manager, "codex", model, 2),
		[]string{"demote-middle", "demote-near"})
}

func TestManagerFillFirst_CodexResetRefreshReordersOnMetadataUpdate(t *testing.T) {
	t.Parallel()

	const model = "codex-reset-refresh-model"
	manager, _ := newFillFirstResetManager(t)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	now := time.Now()
	nearReset := now.Add(time.Hour)
	farReset := now.Add(3 * time.Hour)

	near := codexResetCodexAuth("refresh", "near", &nearReset)
	far := codexResetCodexAuth("refresh", "far", &farReset)
	registerResetAuths(t, manager, "codex", model, near, far)

	assertPickOrder(t, pickNextIDs(t, manager, "codex", model, 1), []string{"refresh-near"})

	// Swap the two reset times through Manager.Update, exactly as the management-side quota
	// refresh does. Nothing else about the credential changes: same state, same priority,
	// same cooldown, same demotion flag. The reordering therefore depends entirely on the
	// cached reset taking part in upsertEntryLocked's short-circuit comparison.
	nearReset, farReset = farReset, nearReset
	near.Metadata = codexResetMetadata(nearReset)
	far.Metadata = codexResetMetadata(farReset)
	if _, errUpdate := manager.Update(context.Background(), near); errUpdate != nil {
		t.Fatalf("Update(refresh-near) error = %v", errUpdate)
	}
	if _, errUpdate := manager.Update(context.Background(), far); errUpdate != nil {
		t.Fatalf("Update(refresh-far) error = %v", errUpdate)
	}

	assertPickOrder(t, pickNextIDs(t, manager, "codex", model, 1), []string{"refresh-far"})
}

func TestManagerFillFirst_NonCodexIgnoresResetMetadata(t *testing.T) {
	t.Parallel()

	const model = "gemini-reset-metadata-model"
	manager, seed := newFillFirstResetManager(t)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	now := time.Now()
	auths := []*Auth{
		{ID: "gemini-meta-a", Provider: "gemini"},
		{ID: "gemini-meta-b", Provider: "gemini"},
		{ID: "gemini-meta-c", Provider: "gemini"},
	}

	// Give the credential with the worst shuffle rank the nearest reset so the test fails
	// if non-Codex credentials ever start honoring Codex quota reset metadata.
	worstIndex := 0
	for index := 1; index < len(auths); index++ {
		if fillFirstShuffleRank(seed, auths[index].ID) > fillFirstShuffleRank(seed, auths[worstIndex].ID) {
			worstIndex = index
		}
	}
	auths[worstIndex].Metadata = map[string]any{
		"codex_quota": map[string]any{
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 50,
					"reset_at":     now.Add(time.Minute).Format(time.RFC3339Nano),
				},
			},
		},
	}

	registerResetAuths(t, manager, "gemini", model, auths...)

	wantFirst := ""
	wantRank := ^uint64(0)
	for _, auth := range auths {
		rank := fillFirstShuffleRank(seed, auth.ID)
		if rank < wantRank || (rank == wantRank && auth.ID < wantFirst) {
			wantFirst = auth.ID
			wantRank = rank
		}
	}
	if wantFirst == auths[worstIndex].ID {
		t.Fatal("test setup is not discriminating: the nearest-reset holder already has the best shuffle rank")
	}

	assertPickOrder(t, pickNextIDs(t, manager, "gemini", model, 1), []string{wantFirst})
}

func TestManagerFillFirst_CodexResetOrdersCapacitySpill(t *testing.T) {
	t.Parallel()

	const model = "codex-reset-spill-model"
	manager, _ := newFillFirstResetManager(t)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})
	manager.SetConfig(&internalconfig.Config{
		Routing: internalconfig.RoutingConfig{
			Strategy:             "fill-first",
			FillFirstMaxInflight: 1,
		},
	})

	now := time.Now()
	nearReset := now.Add(time.Hour)
	middleReset := now.Add(2 * time.Hour)
	farReset := now.Add(3 * time.Hour)

	registerResetAuths(t, manager, "codex", model,
		codexResetCodexAuth("spill", "far", &farReset),
		codexResetCodexAuth("spill", "middle", &middleReset),
		codexResetCodexAuth("spill", "near", &nearReset),
	)

	if got := manager.fillFirstMaxInflight(); got != 1 {
		t.Fatalf("fillFirstMaxInflight() = %d, want 1", got)
	}

	// Saturate the sticky credential exactly as the execution path does, then verify the
	// spill lands on the next-nearest reset rather than on an arbitrary ready credential.
	release := manager.beginFillFirstInflight("spill-near")
	defer release()

	assertPickOrder(t, pickNextIDs(t, manager, "codex", model, 1), []string{"spill-middle"})
}

func TestLessScheduledAuthForCodexReset_OnlyDecidesForCodex(t *testing.T) {
	t.Parallel()

	now := time.Now()
	nearReset := now.Add(time.Hour)
	farReset := now.Add(2 * time.Hour)

	// Only Codex credentials ever get a cached reset, so a populated codexResetAt is itself
	// the provider gate: non-Codex metas keep it zero and are ignored.
	near := &scheduledAuth{meta: &scheduledAuthMeta{codexResetAt: nearReset}, auth: &Auth{ID: "cmp-near", Provider: "codex"}}
	far := &scheduledAuth{meta: &scheduledAuthMeta{codexResetAt: farReset}, auth: &Auth{ID: "cmp-far", Provider: "codex"}}

	less, decided := lessScheduledAuthForCodexReset(near, far, now)
	if !decided || !less {
		t.Fatalf("codex pair: less = %v, decided = %v, want true, true", less, decided)
	}

	// A gemini credential carrying codex_quota metadata still caches nothing, so it must
	// never participate in the ordering.
	gemini := &scheduledAuth{meta: &scheduledAuthMeta{}, auth: &Auth{ID: "cmp-gemini", Provider: "gemini"}}
	if _, decided = lessScheduledAuthForCodexReset(gemini, gemini, now); decided {
		t.Fatal("non-codex pair must leave ordering to the caller's tie-breakers")
	}

	// A cached reset that has already elapsed must degrade to unknown, exactly as a freshly
	// parsed one would, so it cannot keep outranking a live reset after the fact.
	stale := &scheduledAuth{
		meta: &scheduledAuthMeta{codexResetAt: now.Add(-time.Hour)},
		auth: &Auth{ID: "cmp-stale", Provider: "codex"},
	}
	less, decided = lessScheduledAuthForCodexReset(stale, far, now)
	if !decided || less {
		t.Fatalf("stale vs live: less = %v, decided = %v, want false, true", less, decided)
	}
	if _, decided = lessScheduledAuthForCodexReset(stale, stale, now); decided {
		t.Fatal("two stale resets must leave ordering to the caller's tie-breakers")
	}
}
