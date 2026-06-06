package management

import (
	"encoding/json"
	"testing"
)

func TestMinimalCodexRefreshPayloadSetsStoreFalse(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal(minimalCodexRefreshPayload(), &payload); err != nil {
		t.Fatalf("unmarshal minimal payload: %v", err)
	}
	if payload["model"] != "gpt-5.4-mini" {
		t.Fatalf("model = %v, want gpt-5.4-mini", payload["model"])
	}
	store, ok := payload["store"].(bool)
	if !ok {
		t.Fatalf("store field type = %T, want bool", payload["store"])
	}
	if store {
		t.Fatalf("store = true, want false")
	}
}
