package queue

import "testing"

func TestPgTextArrayUsesEmptySliceNotNull(t *testing.T) {
	agents := pgTextArray(nil)
	if agents == nil {
		t.Fatal("expected non-nil empty slice for postgres text[] binding")
	}
	if len(agents) != 0 {
		t.Fatalf("expected empty slice, got %#v", agents)
	}
}

func TestMetadataStringSliceMissingKeyReturnsEmptyArray(t *testing.T) {
	got := metadataStringSlice(map[string]any{}, "external_agents_used")
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %#v", got)
	}
}
