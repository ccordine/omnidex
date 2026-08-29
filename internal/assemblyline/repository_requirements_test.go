package assemblyline

import (
	"strings"
	"testing"
)

func TestResolveRepositoryRequirementsRequiresExactOrderedRequestSpans(t *testing.T) {
	t.Parallel()
	const request = "Add audit logging and CSV exports to the service."
	context, err := BootstrapApplicationContext(
		request, ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := RepositoryRequirementInterpretationInput{UserRequest: request, Context: context}
	resolved, err := ResolveRepositoryRequirements(input, RepositoryRequirementInterpretation{
		Schema:       RepositoryRequirementInterpretationSchemaV3,
		Requirements: []string{"audit logging", "CSV exports"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(resolved, "|") != "audit logging|CSV exports" {
		t.Fatalf("resolved=%q", resolved)
	}
	for name, requirements := range map[string][]string{
		"paraphrase": {"Add CSV export support"},
		"reordered":  {"CSV exports", "audit logging"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveRepositoryRequirements(input, RepositoryRequirementInterpretation{
				Schema:       RepositoryRequirementInterpretationSchemaV3,
				Requirements: requirements,
			})
			if err == nil {
				t.Fatal("ungrounded repository requirement was accepted")
			}
		})
	}
}
