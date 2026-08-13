package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestArtifactHandlingCanStateRequiredAbsenceWithoutOperationAuthority(t *testing.T) {
	t.Parallel()
	decision := ArtifactHandlingDecision{
		Schema: ArtifactHandlingSchemaV1, Token: "ARTIFACT_1", Handling: ArtifactMustBeAbsent,
	}
	if err := decision.Validate("ARTIFACT_1"); err != nil {
		t.Fatal(err)
	}
	input := ArtifactHandlingInput{
		UserRequest: "ARTIFACT_1 must no longer exist.", Token: "ARTIFACT_1",
	}
	prompt, err := BuildArtifactHandlingPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := json.Marshal(ArtifactHandlingResponseSchema(input.Token))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "must_be_absent") || !strings.Contains(string(schema), "must_be_absent") {
		t.Fatalf("required-absence semantic leaf is missing: prompt=%q schema=%s", prompt, schema)
	}
	for _, forbidden := range []string{"create_file", "delete_file", "write_file", "rename_file", "filesystem operation enum"} {
		if strings.Contains(prompt, forbidden) || strings.Contains(string(schema), forbidden) {
			t.Fatalf("artifact boundary exposed forbidden operation %q", forbidden)
		}
	}
}

func TestArtifactHandlingCanStateUnresolvedAbsenceMembershipWithoutMutationAuthority(t *testing.T) {
	t.Parallel()
	input := ArtifactHandlingInput{
		UserRequest: "Remove whichever of ARTIFACT_1 or ARTIFACT_2 owns LegacyAdapter.",
		Token:       "ARTIFACT_1",
	}
	decision := ArtifactHandlingDecision{
		Schema: ArtifactHandlingSchemaV1, Token: input.Token,
		Handling: ArtifactPossibleAbsenceCandidate,
	}
	if err := decision.Validate(input.Token); err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildArtifactHandlingPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := json.Marshal(ArtifactHandlingResponseSchema(input.Token))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, string(ArtifactPossibleAbsenceCandidate)) ||
		!strings.Contains(string(schema), string(ArtifactPossibleAbsenceCandidate)) {
		t.Fatalf("possible absence truth is absent from prompt/schema: %s\n%s", prompt, schema)
	}
	for _, forbidden := range []string{
		"create_file", "delete_file", "write_file", "rename_file", "move_file",
	} {
		if strings.Contains(prompt+string(schema), forbidden) {
			t.Fatalf("truth leaf exposed mutation authority %q", forbidden)
		}
	}
}

func TestApplicationSpecificationRejectsUnknownArtifactDisposition(t *testing.T) {
	t.Parallel()
	specification := ApplicationSpecification{
		Surface: ApplicationSurfaceBrowser, ProductQuote: "browser utility",
		Requirements: []Requirement{{ID: "requirement_001", SourceQuote: "show records"}},
		Artifacts:    []ArtifactDirective{{Token: "ARTIFACT_1", Disposition: "delete_file"}},
	}
	if err := specification.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported disposition") {
		t.Fatalf("unknown operation-like disposition error=%v", err)
	}
}
