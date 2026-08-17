package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestApplicationContextBootstrapRecordsOnlyCodeAndAcceptedMemoryAuthority(t *testing.T) {
	t.Parallel()
	request := "Build a browser counter with increment and reset controls."
	memory := ObjectiveMemoryAuthority{
		MemoryID: 17, Kind: model.MemoryKindReference,
		Content:       "Prefer accessible native controls.",
		ContentSHA256: ExactObjectiveContextSHA("Prefer accessible native controls."),
	}
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty, []ObjectiveMemoryAuthority{memory})
	if err != nil {
		t.Fatal(err)
	}
	if context.WorkspaceState != ApplicationWorkspaceEmpty ||
		context.RequestSHA256 != ExactObjectiveContextSHA(request) || len(context.Facts) != 2 {
		t.Fatalf("context=%+v", context)
	}
	if context.Facts[0].Kind != ApplicationContextWorkspaceState ||
		context.Facts[0].Authority != ApplicationContextCodeAuthority || context.Facts[0].Value != "empty" {
		t.Fatalf("workspace fact=%+v", context.Facts[0])
	}
	if context.Facts[1].Kind != ApplicationContextAcceptedMemory ||
		context.Facts[1].Authority != ApplicationContextMemoryAuthority ||
		context.Facts[1].Value != memory.Content || context.Facts[1].SourceSHA256 != memory.ContentSHA256 {
		t.Fatalf("memory fact=%+v", context.Facts[1])
	}
}

func TestApplicationContextNeedStationReturnsQuestionsNotTools(t *testing.T) {
	t.Parallel()
	request := "Build a browser counter with increment and reset controls."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty, nil)
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewApplicationContextNeedJob(ApplicationContextNeedInput{
		UserRequest: request, Context: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, request) != 1 {
		t.Fatalf("request count=%d prompt=%q", strings.Count(prompt, request), prompt)
	}
	for _, forbidden := range []string{"search_repo", "search_web", "search_memory", "read_file", "tool_call", "TOOLS:"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("context-need prompt exposes tool surface %q: %s", forbidden, prompt)
		}
	}
	properties := schema["properties"].(map[string]any)
	questions := properties["questions"].(map[string]any)
	if questions["minItems"] != 0 || questions["maxItems"] != MaxApplicationEvidenceNeeds {
		t.Fatalf("questions schema=%#v", questions)
	}
	decision, err := DecodeApplicationContextNeedDecision(
		ApplicationContextNeedInput{UserRequest: request, Context: context},
		`{"schema":"omnidex.application-context-needs.v1","questions":[]}`,
	)
	if err != nil || len(decision.Questions) != 0 {
		t.Fatalf("zero-need decision=%+v err=%v", decision, err)
	}
}

func TestApplicationIntentUsesReviewedSemanticStatementsWithoutSubstringGates(t *testing.T) {
	t.Parallel()
	request := "Build a browser-based counter app that displays the current count and has buttons to increment it, decrement it, and reset it to zero."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationIntentInput{UserRequest: request, Context: context}
	candidate := ApplicationIntentCandidate{
		Schema:         ApplicationIntentCandidateSchemaV1,
		ProductContext: "A browser counter application",
		Requirements: []string{
			"Show the current count.",
			"Provide controls that increment, decrement, and reset the count.",
		},
	}
	resolved, err := ResolveApplicationIntent(input, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProductContext != candidate.ProductContext || resolved.RequestSHA256 != context.RequestSHA256 {
		t.Fatalf("resolution=%+v", resolved)
	}
	want := []ApplicationRequirement{
		{ID: "requirement_001", Statement: candidate.Requirements[0], RequestSHA256: context.RequestSHA256},
		{ID: "requirement_002", Statement: candidate.Requirements[1], RequestSHA256: context.RequestSHA256},
	}
	if !reflect.DeepEqual(resolved.Requirements, want) {
		t.Fatalf("requirements=%+v want=%+v", resolved.Requirements, want)
	}
	for _, statement := range candidate.Requirements {
		if strings.Contains(request, statement) {
			t.Fatalf("fixture unexpectedly used an exact substring: %q", statement)
		}
	}
}

func TestApplicationIntentReviewReturnsOneBoundReplacementValue(t *testing.T) {
	t.Parallel()
	input := applicationIntentContractInput(t)
	candidate := ApplicationIntentCandidate{
		Schema:         ApplicationIntentCandidateSchemaV1,
		ProductContext: "A browser counter",
		Requirements:   []string{"Show the count.", "Provide increment and reset controls."},
	}
	reviewInput := ApplicationIntentReviewInput{Authority: input, Candidate: candidate, Target: "requirements_002"}
	job, err := NewApplicationIntentReviewJob(reviewInput)
	if err != nil {
		t.Fatal(err)
	}
	_, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	if _, exists := properties["finding"]; exists {
		t.Fatal("review schema exposes a model-to-model finding protocol")
	}
	if properties["replacement_value"] == nil {
		t.Fatal("review schema omits direct replacement value")
	}
}

func TestApplicationIntentReviewCandidatesAreSplicedByCode(t *testing.T) {
	t.Parallel()
	input := applicationIntentContractInput(t)
	retained := ApplicationIntentCandidate{
		Schema:         ApplicationIntentCandidateSchemaV1,
		ProductContext: "A browser counter",
		Requirements:   []string{"Show the count.", "Provide increment and reset controls."},
	}
	reviewInput := ApplicationIntentReviewInput{Authority: input, Candidate: retained, Target: "requirements_002"}
	review, err := DecodeApplicationIntentReview(
		reviewInput,
		`{"schema":"omnidex.application-intent-review.v1","decision":"replace","replacement_value":"Provide controls that increment, decrement, and reset the count."}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := ApplyApplicationIntentReviewReplacement(input, retained, review)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.ProductContext != retained.ProductContext ||
		corrected.Requirements[0] != retained.Requirements[0] ||
		corrected.Requirements[1] == retained.Requirements[1] {
		t.Fatalf("repair changed the wrong authority: before=%+v after=%+v", retained, corrected)
	}
}

func applicationIntentContractInput(t *testing.T) ApplicationIntentInput {
	t.Helper()
	request := "Build a browser counter that shows the count and can increment, decrement, and reset it."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationIntentInput{UserRequest: request, Context: context}
}
