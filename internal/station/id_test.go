package station

import "testing"

func TestIDRejectsUnregisteredSemanticAuthority(t *testing.T) {
	t.Parallel()

	if err := ID("planner_specialist").Validate(); err == nil {
		t.Fatal("persona-shaped station ID was accepted")
	}
	if err := CodingFragment.Validate(); err != nil {
		t.Fatalf("registered leaf station rejected: %v", err)
	}
	if err := ObjectiveAdvisory.Validate(); err != nil {
		t.Fatalf("registered passive advisory source rejected: %v", err)
	}
}

func TestAllContainsOnlyUniqueRegisteredStations(t *testing.T) {
	t.Parallel()

	seen := map[ID]struct{}{}
	for _, id := range All() {
		if err := id.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate station %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 35 {
		t.Fatalf("registered stations=%d want 35", len(seen))
	}
}
