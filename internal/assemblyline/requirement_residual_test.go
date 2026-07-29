package assemblyline

import (
	"strings"
	"testing"
)

func TestBuildRequirementResidualMasksOnlyGroundedCoveredSpans(t *testing.T) {
	t.Parallel()

	source := "Build a browser tool with a counter,\nsummary, and reset control."
	residual, err := BuildRequirementResidual(source, []string{"a counter", "reset control"})
	if err != nil {
		t.Fatal(err)
	}
	if len(residual) != len(source) {
		t.Fatalf("residual length = %d, want %d", len(residual), len(source))
	}
	if strings.Contains(residual, "a counter") || strings.Contains(residual, "reset control") {
		t.Fatalf("covered spans remained visible: %q", residual)
	}
	if !strings.Contains(residual, "summary") || !strings.Contains(residual, "\n") {
		t.Fatalf("uncovered source text or layout was lost: %q", residual)
	}
	for index := range source {
		if residual[index] != ' ' && residual[index] != source[index] {
			t.Fatalf("residual byte %d = %q, source = %q", index, residual[index], source[index])
		}
	}
}

func TestBuildRequirementResidualRejectsAmbiguousAndOverlappingSpans(t *testing.T) {
	t.Parallel()

	for name, quotes := range map[string][]string{
		"ambiguous": {"control"},
		"overlap":   {"a reset control", "reset control"},
		"missing":   {"dial"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := BuildRequirementResidual("a control and a reset control", quotes)
			if err == nil {
				t.Fatalf("accepted invalid covered spans %#v", quotes)
			}
		})
	}
}
