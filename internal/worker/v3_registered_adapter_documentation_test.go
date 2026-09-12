package worker

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestRegisteredCodingAdaptersMatchDocumentation(t *testing.T) {
	document, err := os.ReadFile("../../docs/ARTIFACT_ADAPTERS.md")
	if err != nil {
		t.Fatal(err)
	}
	var adapters, stacks, profiles []string
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		adapters = append(adapters, fmt.Sprintf("| `%s` | `%s` |", adapter.ID, adapter.Validation.Kind))
	}
	for _, stack := range registeredDirectCodingProjectStacks() {
		stacks = append(stacks, fmt.Sprintf("| `%s` | `%s` |", stack.ID,
			directCodingProjectStackSurfaceSummary(stack.SupportedSurfaces)))
	}
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		dialect, err := directCodingProjectSourceDialect(profile)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, fmt.Sprintf("| `%s` | `%s` | `%s` |",
			profile.ID, profile.StackID, dialect))
	}
	for _, section := range []struct {
		name, heading, separator string
		rows                     []string
	}{
		{"ARTIFACT_ADAPTER_REGISTRY", "| Adapter | Executable leaf validation |",
			"| --- | --- |", adapters},
		{"PROJECT_STACK_REGISTRY", "| Stack | Supported surfaces |",
			"| --- | --- |", stacks},
		{"PROJECT_VERSION_PROFILE_REGISTRY", "| Version profile | Stack | Source dialect |",
			"| --- | --- | --- |", profiles},
	} {
		t.Run(section.name, func(t *testing.T) {
			begin, end := "<!-- BEGIN "+section.name+" -->", "<!-- END "+section.name+" -->"
			text := string(document)
			if strings.Count(text, begin) != 1 || strings.Count(text, end) != 1 {
				t.Fatal("documentation must contain exactly one bounded registry table")
			}
			_, afterBegin, _ := strings.Cut(text, begin)
			actual, _, found := strings.Cut(afterBegin, end)
			if !found {
				t.Fatal("documentation registry table closes before it opens")
			}
			sort.Strings(section.rows)
			expected := "\n" + section.heading + "\n" + section.separator + "\n" +
				strings.Join(section.rows, "\n") + "\n"
			if actual != expected {
				t.Errorf("documented registry differs from executable registrations\nactual:%s\nexpected:%s",
					actual, expected)
			}
		})
	}
}
