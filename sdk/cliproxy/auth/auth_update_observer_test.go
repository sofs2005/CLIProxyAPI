package auth

import (
	"context"
	"testing"
)

func TestAuthUpdateObserverReceivesBeforeAndAfterSnapshots(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-update-observer-test",
		Provider: "xai",
		Attributes: map[string]string{
			AttributeAuthKind: AuthKindOAuth,
		},
	}
	if _, errRegister := manager.Register(context.Background(), auth); errRegister != nil {
		t.Fatal(errRegister)
	}

	var previous, current *Auth
	calls := 0
	manager.AddAuthUpdateObserver(func(_ context.Context, before, after *Auth) {
		calls++
		previous = before
		current = after
	})

	updated := auth.Clone()
	updated.Attributes[AttributeXAIBFS] = "true"
	if _, errUpdate := manager.Update(context.Background(), updated); errUpdate != nil {
		t.Fatal(errUpdate)
	}
	if calls != 1 {
		t.Fatalf("observer calls = %d, want 1", calls)
	}
	if previous == nil || previous.Attributes[AttributeXAIBFS] != "" {
		t.Fatalf("previous snapshot = %#v, want no BFS marker", previous)
	}
	if current == nil || current.Attributes[AttributeXAIBFS] != "true" {
		t.Fatalf("current snapshot = %#v, want BFS marker", current)
	}

	if _, errUpdate := manager.Update(context.Background(), updated); errUpdate != nil {
		t.Fatal(errUpdate)
	}
	if calls != 2 {
		t.Fatalf("observer calls after unchanged update = %d, want 2", calls)
	}
}
