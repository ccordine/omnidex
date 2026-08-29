package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplicationContextBootstrapRecordsOnlyCurrentWorkspaceAuthority(t *testing.T) {
	t.Parallel()
	request := "Build a browser counter with increment and reset controls."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if context.WorkspaceState != ApplicationWorkspaceEmpty ||
		context.RequestSHA256 != ExactObjectiveContextSHA(request) || len(context.Facts) != 1 {
		t.Fatalf("context=%+v", context)
	}
	if context.Facts[0].Kind != ApplicationContextWorkspaceState ||
		context.Facts[0].Authority != ApplicationContextCodeAuthority || context.Facts[0].Value != "empty" {
		t.Fatalf("workspace fact=%+v", context.Facts[0])
	}
}

func TestApplicationContextRejectsHistoricalMemoryFacts(t *testing.T) {
	t.Parallel()
	context, err := BootstrapApplicationContext(
		"Build a browser counter with increment and reset controls.",
		ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	memory := "Prior preference that is not current coding authority."
	context.Facts = append(context.Facts, ApplicationContextFact{
		ID: "fact_002", Kind: ApplicationContextFactKind("accepted_memory"),
		Authority: ApplicationContextAuthority("accepted_memory"), Value: memory,
		SourceID: "memory_17", SourceSHA256: ExactObjectiveContextSHA(memory),
	})
	if err := context.Validate(); err == nil || !strings.Contains(err.Error(), "kind \"accepted_memory\" is unsupported") {
		t.Fatalf("historical memory fact validation error=%v", err)
	}
}

func TestApplicationContextNeedStationReturnsQuestionsNotTools(t *testing.T) {
	t.Parallel()
	request := "Build a browser counter with increment and reset controls."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	leafInput := ApplicationContextNeedLeafInput{
		UserRequest: request, Context: context, AcceptedQuestions: []string{},
	}
	job, err := NewApplicationContextNeedCoverageJob(leafInput)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
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
	coverage, err := DecodeApplicationContextNeedCoverageLeaf(
		leafInput, ApplicationNoUncoveredContextNeed,
	)
	if err != nil || coverage != ApplicationNoUncoveredContextNeed {
		t.Fatalf("coverage=%q err=%v", coverage, err)
	}
	decision, err := AssembleApplicationContextNeedDecision(ApplicationContextNeedInput{
		UserRequest: request, Context: context,
	}, []string{})
	if err != nil || len(decision.Questions) != 0 {
		t.Fatalf("zero-need decision=%+v err=%v", decision, err)
	}
	questionJob, err := NewApplicationContextNeedQuestionJob(ApplicationContextNeedLeafInput{
		UserRequest: request, Context: context, AcceptedQuestions: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	questionPrompt, err := RenderPortableJob(questionJob)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(questionPrompt, request) != 1 {
		t.Fatalf("question prompt=%q", questionPrompt)
	}
}

func TestApplicationIntentUsesReviewedSemanticStatementsWithoutSubstringGates(t *testing.T) {
	t.Parallel()
	request := "Build a browser-based counter app that displays the current count and has buttons to increment it, decrement it, and reset it to zero."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationIntentInput{UserRequest: request, Context: context}
	statements := []string{
		"Show the current count.",
		"Provide controls that increment, decrement, and reset the count.",
	}
	candidate := ApplicationIntentCandidate{
		Schema:         ApplicationIntentCandidateSchemaV1,
		ProductContext: "A browser counter application",
		Requirements: []ApplicationIntentCandidateRequirement{
			applicationIntentCandidateRequirementFixture(
				t,
				statements[0], ApplicationRequirementNoDerivedResult,
			),
			applicationIntentCandidateRequirementFixture(
				t,
				statements[1], ApplicationRequirementExplicitResultRelation,
			),
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
		{
			ID: "requirement_001", Statement: statements[0], RequestSHA256: context.RequestSHA256,
			ResultRelation: candidate.Requirements[0].ResultRelation,
		},
		{
			ID: "requirement_002", Statement: statements[1], RequestSHA256: context.RequestSHA256,
			ResultRelation: candidate.Requirements[1].ResultRelation,
		},
	}
	if !reflect.DeepEqual(resolved.Requirements, want) {
		t.Fatalf("requirements=%+v want=%+v", resolved.Requirements, want)
	}
	for _, statement := range statements {
		if strings.Contains(request, statement) {
			t.Fatalf("fixture unexpectedly used an exact substring: %q", statement)
		}
	}
}

func applicationIntentCandidateRequirementFixture(
	t testing.TB,
	statement string,
	relation string,
) ApplicationIntentCandidateRequirement {
	t.Helper()
	result, err := DecodeApplicationRequirementCandidateResultRelationResult(
		applicationRequirementCandidateResultRelationInputFixture(t, statement),
		relation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationIntentCandidateRequirement{
		Statement: statement, ResultRelation: result,
	}
}
