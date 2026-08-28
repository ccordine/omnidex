package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationTaskAuthorityProjectionsSeparateRuntimeAndVerification(t *testing.T) {
	t.Parallel()
	input, frozen := applicationTaskAuthorityProjectionFixture(t)
	originalHash := frozen.SHA256

	runtime, err := ProjectApplicationTaskRuntimeAuthority(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	verification, err := ProjectApplicationTaskVerificationAuthority(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}

	runtimeJSON := marshalTaskAuthorityProjection(t, runtime)
	for _, required := range []string{
		string(input.Surface), input.ProductQuote, input.Requirements[0].SourceQuote,
		frozen.Tasks[0].Objective, frozen.Tasks[0].RequiredBehaviors[0],
	} {
		if !strings.Contains(runtimeJSON, required) {
			t.Fatalf("runtime authority omitted %q: %s", required, runtimeJSON)
		}
	}
	for _, forbidden := range []string{
		frozen.Tasks[0].AcceptanceCriteria[0], `"acceptance_criteria"`,
	} {
		if strings.Contains(runtimeJSON, forbidden) {
			t.Fatalf("runtime authority exposed verification value %q: %s", forbidden, runtimeJSON)
		}
	}

	verificationJSON := marshalTaskAuthorityProjection(t, verification)
	if !strings.Contains(verificationJSON, frozen.Tasks[0].AcceptanceCriteria[0]) {
		t.Fatalf("verification authority omitted criterion: %s", verificationJSON)
	}
	for _, forbidden := range []string{
		input.ProductQuote, input.Requirements[0].SourceQuote, frozen.Tasks[0].Objective,
		frozen.Tasks[0].RequiredBehaviors[0], `"surface"`, `"product_quote"`,
		`"requirement_quote"`, `"objective"`, `"required_behaviors"`,
	} {
		if strings.Contains(verificationJSON, forbidden) {
			t.Fatalf("verification authority exposed runtime value %q: %s", forbidden, verificationJSON)
		}
	}

	if frozen.SHA256 != originalHash {
		t.Fatalf("authority projection changed frozen workload hash: got %s want %s", frozen.SHA256, originalHash)
	}
	if err := ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		t.Fatalf("authority projection changed frozen workload: %v", err)
	}
}

func TestApplicationTaskAuthorityProjectionsDefensivelyCopySlices(t *testing.T) {
	t.Parallel()
	input, frozen := applicationTaskAuthorityProjectionFixture(t)
	runtime, err := ProjectApplicationTaskRuntimeAuthority(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	verification, err := ProjectApplicationTaskVerificationAuthority(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}

	runtime.RequiredBehaviors[0] = "mutated runtime projection"
	verification.AcceptanceCriteria[0] = "mutated verification projection"
	if frozen.Tasks[0].RequiredBehaviors[0] == runtime.RequiredBehaviors[0] ||
		frozen.Tasks[0].AcceptanceCriteria[0] == verification.AcceptanceCriteria[0] {
		t.Fatal("authority projection aliases frozen task slices")
	}

	runtime, err = ProjectApplicationTaskRuntimeAuthority(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	verification, err = ProjectApplicationTaskVerificationAuthority(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	frozen.Tasks[0].RequiredBehaviors[0] = "mutated frozen runtime value"
	frozen.Tasks[0].AcceptanceCriteria[0] = "mutated frozen verification value"
	if runtime.RequiredBehaviors[0] == frozen.Tasks[0].RequiredBehaviors[0] ||
		verification.AcceptanceCriteria[0] == frozen.Tasks[0].AcceptanceCriteria[0] {
		t.Fatal("frozen task slices alias authority projections")
	}
}

func TestApplicationTaskAuthorityProjectionsRejectUnboundTask(t *testing.T) {
	t.Parallel()
	input, frozen := applicationTaskAuthorityProjectionFixture(t)
	if _, err := ProjectApplicationTaskRuntimeAuthority(input, frozen, "task_999"); err == nil {
		t.Fatal("runtime authority accepted an unbound task")
	}
	if _, err := ProjectApplicationTaskVerificationAuthority(input, frozen, "task_999"); err == nil {
		t.Fatal("verification authority accepted an unbound task")
	}
}

func applicationTaskAuthorityProjectionFixture(
	t *testing.T,
) (ApplicationWorkloadDraftInput, FrozenApplicationWorkload) {
	t.Helper()
	input := ApplicationWorkloadDraftInput{
		Surface: ApplicationSurfaceService, ProductQuote: "sentinel inventory service",
		Requirements: []Requirement{{
			ID: "requirement_001", SourceQuote: "Runtime requirement sentinel accepts one inventory record.",
		}},
	}
	draft := ApplicationWorkloadDraft{
		Schema: ApplicationWorkloadDraftSchemaV1,
		Tasks: []ApplicationWorkloadTaskDraft{{
			RequirementID: "requirement_001",
			Objective:     "Runtime objective sentinel retains the accepted record.",
			RequiredBehaviors: []string{
				"Runtime behavior sentinel stores the accepted record.",
			},
			AcceptanceCriteria: []string{
				"Verification criterion sentinel observes the record later.",
			},
		}},
	}
	frozen, err := FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	return input, frozen
}

func marshalTaskAuthorityProjection(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(repeated) {
		t.Fatalf("projection JSON is not deterministic: %s", raw)
	}
	return string(raw)
}
