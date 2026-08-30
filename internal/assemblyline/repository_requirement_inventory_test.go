package assemblyline

import (
	"strings"
	"testing"
)

func TestRepositoryRequirementInventoryPreservesRawSourceOrder(t *testing.T) {
	t.Parallel()
	input := repositoryRequirementInventoryTestInput(t)
	raw := strings.Join([]string{
		"Add audit logging.",
		"The service is old.",
		"Add CSV export.",
	}, "\n")
	inventory, err := DecodeRepositoryRequirementInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Schema != RepositoryRequirementInventorySchemaV1 ||
		len(inventory.Candidates) != 3 ||
		strings.Join(inventory.Candidates, "\n") != raw {
		t.Fatalf("inventory=%+v", inventory)
	}
	if err := inventory.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryRequirementInventoryPromptSeesOnlyImmutableRequest(t *testing.T) {
	t.Parallel()
	input := repositoryRequirementInventoryTestInput(t)
	need, err := NewApplicationRepositoryContextNeed(1, "Which declaration owns audit logging?")
	if err != nil {
		t.Fatal(err)
	}
	const fact = "AuditWriter owns the current logging path."
	input.Context, err = AppendApplicationContextEvidence(input.Context, need, []ApplicationContextEvidence{{
		Value: fact, SourceID: "symbol:AuditWriter", SourceSHA256: ExactObjectiveContextSHA(fact),
	}})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildRepositoryRequirementInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.UserRequest) {
		t.Fatalf("inventory prompt omitted immutable request:\n%s", prompt)
	}
	for _, forbidden := range []string{fact, "WORKSPACE STATE:"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("inventory prompt exposed non-request context %q:\n%s", forbidden, prompt)
		}
	}
}

func TestRepositoryRequirementInventoryRejectsNonSourceAndOutOfOrderLines(t *testing.T) {
	t.Parallel()
	input := repositoryRequirementInventoryTestInput(t)
	for name, raw := range map[string]string{
		"paraphrase": "Create logging.",
		"reverse order": strings.Join([]string{
			"Add CSV export.",
			"Add audit logging.",
		}, "\n"),
		"json":       `["Add audit logging."]`,
		"blank line": "Add audit logging.\n\nAdd CSV export.",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeRepositoryRequirementInventory(input, raw); err == nil {
				t.Fatalf("inventory %q was accepted", raw)
			}
		})
	}
}

func TestRepositoryRequirementCandidateAuthorizationBindsOneInventoryCandidate(t *testing.T) {
	t.Parallel()
	input := repositoryRequirementInventoryTestInput(t)
	inventory, err := DecodeRepositoryRequirementInventory(
		input,
		"Add audit logging.\nThe service is old.\nAdd CSV export.",
	)
	if err != nil {
		t.Fatal(err)
	}
	relationInput := RepositoryRequirementCandidateAuthorizationInput{
		Authority: input, Inventory: inventory, CandidateIndex: 1,
	}
	job, err := NewRepositoryRequirementCandidateAuthorizationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "EXACT SOURCE CLAUSE CANDIDATE:\nThe service is old.") ||
		strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
		t.Fatalf("candidate relation prompt exposed wrong authority:\n%s", prompt)
	}
	result, err := DecodeRepositoryRequirementCandidateAuthorizationResult(
		relationInput,
		RepositoryRequirementCandidateNoChange,
	)
	if err != nil || result.Relation != RepositoryRequirementCandidateNoChange {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	drifted := relationInput
	drifted.CandidateIndex = 0
	if err := result.ValidateFor(drifted); err == nil {
		t.Fatal("candidate authorization receipt accepted another inventory candidate")
	}
}

func TestRepositoryRequirementCandidateRelationCannotReopenRetainedState(t *testing.T) {
	t.Parallel()
	input := RepositoryRequirementCandidateRelationInput{
		Candidate:           "Include an audit trail.",
		AcceptedRequirement: "Add audit logging.",
	}
	job, err := NewRepositoryRequirementCandidateRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CANDIDATE CLAUSE:\n" + input.Candidate,
		"ALREADY RETAINED REQUIREMENT:\n" + input.AcceptedRequirement,
		RepositoryRequirementCandidatesSameChange,
		RepositoryRequirementCandidatesDistinctChanges,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("pair relation prompt omitted %q:\n%s", required, prompt)
		}
	}
	result, err := DecodeRepositoryRequirementCandidateRelationResult(
		input,
		RepositoryRequirementCandidatesSameChange,
	)
	if err != nil {
		t.Fatal(err)
	}
	drifted := input
	drifted.AcceptedRequirement = "Add CSV export."
	if err := result.ValidateFor(drifted); err == nil {
		t.Fatal("pair relation receipt accepted another retained requirement")
	}
}

func repositoryRequirementInventoryTestInput(
	t *testing.T,
) RepositoryRequirementInterpretationInput {
	t.Helper()
	request := "Add audit logging. The service is old. Add CSV export."
	context, err := BootstrapApplicationContext(
		request,
		ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	return RepositoryRequirementInterpretationInput{
		UserRequest: request,
		Context:     context,
	}
}
