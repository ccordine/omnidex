package assemblyline

import (
	"strings"
	"testing"
)

func applicationContextQuestionInventoryFixture(
	t testing.TB,
) (ApplicationContextQuestionInventoryInput, ApplicationContextQuestionInventory) {
	t.Helper()
	const request = "Exclude archived patients from the existing search."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceExisting)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationContextQuestionInventoryInput{
		UserRequest: request,
		Context:     context,
	}
	inventory, err := DecodeApplicationContextQuestionInventory(
		input,
		"Which declaration owns archived-patient filtering?\nWhich policy governs visibility of those results?",
	)
	if err != nil {
		t.Fatal(err)
	}
	return input, inventory
}

func TestApplicationContextQuestionInventoryIsOneBoundedUntrustedCollection(t *testing.T) {
	t.Parallel()
	input, inventory := applicationContextQuestionInventoryFixture(t)
	job, err := NewApplicationContextQuestionInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationContextQuestionInventory || len(inventory.Candidates) != 2 {
		t.Fatalf("job=%q inventory=%+v", job.Kind, inventory)
	}
	prompt, err := BuildApplicationContextQuestionInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.UserRequest) ||
		strings.Contains(prompt, input.Context.Facts[0].SourceSHA256) {
		t.Fatalf("application context question inventory crossed authority: %s", prompt)
	}
	absence, err := DecodeApplicationContextQuestionInventory(
		input,
		ApplicationNoRepositoryFactQuestionCandidates,
	)
	if err != nil || absence.Candidates == nil || len(absence.Candidates) != 0 {
		t.Fatalf("absence=%+v err=%v", absence, err)
	}
	duplicate, err := DecodeApplicationContextQuestionInventory(
		input,
		"Which declaration owns archived-patient filtering?\nWhich declaration owns archived-patient filtering?",
	)
	if err != nil || len(duplicate.Candidates) != 2 {
		t.Fatalf("untrusted duplicate inventory=%+v err=%v", duplicate, err)
	}
	emptyContext, err := BootstrapApplicationContext(
		input.UserRequest,
		ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewApplicationContextQuestionInventoryJob(
		ApplicationContextQuestionInventoryInput{
			UserRequest: input.UserRequest,
			Context:     emptyContext,
		},
	); err == nil {
		t.Fatal("fresh empty workspace admitted a ceremonial context-question inventory call")
	}
}

func TestApplicationContextQuestionNecessityBindsEvolvingEvidenceAuthority(t *testing.T) {
	t.Parallel()
	authority, inventory := applicationContextQuestionInventoryFixture(t)
	first := ApplicationContextQuestionNecessityInput{
		Authority: authority, Inventory: inventory, CandidateIndex: 0,
		CurrentContext: authority.Context,
	}
	firstPrompt, err := BuildApplicationContextQuestionNecessityPrompt(first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(firstPrompt, inventory.Candidates[0]) ||
		strings.Contains(firstPrompt, inventory.Candidates[1]) {
		t.Fatalf("necessity prompt exposed candidates outside the focused prefix: %s", firstPrompt)
	}
	result, err := DecodeApplicationContextQuestionNecessityResult(
		first,
		ApplicationContextQuestionNecessary,
	)
	if err != nil || result.Relation != ApplicationContextQuestionNecessary {
		t.Fatalf("first necessity=%+v err=%v", result, err)
	}
	need, err := NewApplicationRepositoryContextNeed(1, inventory.Candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	const fact = "PatientQuery owns archived-patient filtering."
	current, err := AppendApplicationContextEvidence(
		authority.Context,
		need,
		[]ApplicationContextEvidence{{
			Value: fact, SourceID: "symbol:PatientQuery",
			SourceSHA256: ExactObjectiveContextSHA(fact),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	second := ApplicationContextQuestionNecessityInput{
		Authority: authority, Inventory: inventory, CandidateIndex: 1,
		CurrentContext: current,
	}
	prompt, err := BuildApplicationContextQuestionNecessityPrompt(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{fact, inventory.Candidates[1]} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("necessity prompt omitted evolving authority %q: %s", visible, prompt)
		}
	}
	if strings.Contains(prompt, inventory.Candidates[0]) ||
		strings.Contains(prompt, "semantically distinct") ||
		strings.Contains(prompt, "ALREADY ACCEPTED") {
		t.Fatalf("necessity prompt retained pairwise identity responsibility: %s", prompt)
	}
	if _, err := DecodeApplicationContextQuestionNecessityResult(
		second,
		ApplicationContextQuestionNotNecessary,
	); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationContextQuestionRelationSeesExactlyOneImmutablePair(t *testing.T) {
	t.Parallel()
	input := ApplicationContextQuestionRelationInput{
		CandidateQuestion: "What existing symbol is responsible for archived-patient filtering?",
		AcceptedQuestion:  "Which declaration owns archived-patient filtering?",
	}
	job, err := NewApplicationContextQuestionRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{input.CandidateQuestion, input.AcceptedQuestion} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("pairwise relation omitted %q: %s", visible, prompt)
		}
	}
	for _, forbidden := range []string{
		"current established facts", "necessary, still-unresolved", "accepted questions",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("pairwise relation gained another responsibility %q: %s", forbidden, prompt)
		}
	}
	result, err := DecodeApplicationContextQuestionRelationResult(
		input, ApplicationContextQuestionsSameFact,
	)
	if err != nil || result.Relation != ApplicationContextQuestionsSameFact {
		t.Fatalf("relation=%+v err=%v", result, err)
	}
	other := input
	other.CandidateQuestion = "Which policy governs visibility of archived patients?"
	if err := result.ValidateFor(other); err == nil || !strings.Contains(err.Error(), "authority hash") {
		t.Fatalf("cross-pair relation error=%v", err)
	}
	if _, err := NewApplicationContextQuestionRelationJob(ApplicationContextQuestionRelationInput{
		CandidateQuestion: input.AcceptedQuestion,
		AcceptedQuestion:  input.AcceptedQuestion,
	}); err == nil {
		t.Fatal("exact duplicate reached the semantic relation")
	}
}

func TestApplicationContextQuestionInventoryRejectsStructuredAndInvalidCandidates(t *testing.T) {
	t.Parallel()
	input, _ := applicationContextQuestionInventoryFixture(t)
	for _, raw := range []string{
		`{"questions":["Which declaration owns filtering?"]}`,
		`"Which declaration owns filtering?"`,
		"Name the owner",
		"Which declaration owns filtering?\n\nWhich policy governs it?",
		"What owns C:\\private\\value?",
	} {
		if _, err := DecodeApplicationContextQuestionInventory(input, raw); err == nil {
			t.Fatalf("invalid application context question inventory accepted: %q", raw)
		}
	}
}
