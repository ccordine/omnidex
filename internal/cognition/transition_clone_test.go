package cognition

import (
	"encoding/json"
	"testing"
)

func TestTransitionClonePreservesExplicitEmptyCollections(t *testing.T) {
	revision, err := NewWorldRevision(
		"episode-clone", 1,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)
	if err != nil {
		t.Fatal(err)
	}
	original := Transition{
		Current: revision, Observations: []Observation{}, Effects: []Effect{},
	}
	cloned := original.Clone()
	if cloned.Observations == nil || cloned.Effects == nil {
		t.Fatalf("clone collapsed explicit arrays: %+v", cloned)
	}
	raw, err := json.Marshal(cloned)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" || containsJSONNullCollection(raw, "observations") ||
		containsJSONNullCollection(raw, "effects") {
		t.Fatalf("clone JSON lost explicit arrays: %s", raw)
	}
}

func containsJSONNullCollection(raw []byte, field string) bool {
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return true
	}
	value, exists := decoded[field]
	return !exists || value == nil
}
