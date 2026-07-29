package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildRequirementGraphAssignsCodeOwnedStableIDs(t *testing.T) {
	t.Parallel()

	request := "Build a browser inventory with grouped records and a quick filter."
	graph, err := BuildRequirementGraph(request, []string{"grouped records", "a quick filter"})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Requirements) != 2 || graph.Requirements[0].ID != "requirement_001" ||
		graph.Requirements[1].ID != "requirement_002" {
		t.Fatalf("requirements=%#v", graph.Requirements)
	}
}

func TestBuildRequirementGraphRejectsUngroundedDuplicateAndOverlappingQuotes(t *testing.T) {
	t.Parallel()

	request := "Build a record editor with a reset control."
	for name, quotes := range map[string][]string{
		"ungrounded": {"cloud history"},
		"duplicate":  {"reset control", "reset control"},
		"overlap":    {"a reset control", "reset control"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildRequirementGraph(request, quotes); err == nil {
				t.Fatalf("accepted invalid source quotes %#v", quotes)
			}
		})
	}
}

func TestRequirementCarriesNoModelInventedOrConstructionAuthority(t *testing.T) {
	t.Parallel()

	requirementType := reflect.TypeOf(Requirement{})
	for _, forbidden := range []string{
		"Kind", "Outcome", "Path", "File", "Document", "Package", "Module", "Signature",
		"Dependency", "Worker", "Model", "Prompt", "Test", "Repair",
	} {
		if _, exists := requirementType.FieldByName(forbidden); exists {
			t.Fatalf("requirement exposes forbidden field %q", forbidden)
		}
	}
	if _, err := BuildRequirementGraph("Build a tool.", nil); err == nil ||
		!strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty graph did not fail explicitly: %v", err)
	}
}
