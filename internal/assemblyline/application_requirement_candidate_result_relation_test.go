package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateResultRelationIsOneBoundQuestion(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name, candidate, relation string
	}{
		{
			name:      "ordered words",
			candidate: "Sort the user-provided words in ascending Unicode code-point order and display the ordered words.",
			relation:  ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "inventory status",
			candidate: "Display the current inventory status heading.",
			relation:  ApplicationRequirementNoDerivedResult,
		},
		{
			name:      "underdetermined recommendation",
			candidate: "Accept a user's preferences and display the best recommendation.",
			relation:  ApplicationRequirementMissingResultRelation,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			input := applicationRequirementCandidateResultRelationInputFixture(
				t, fixture.candidate,
			)
			prompt, err := BuildApplicationRequirementCandidateResultRelationPrompt(input)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"one semantic classification",
				"precisely enough for an independent test to compute the expected result",
				"semantically entails exactly one determining rule",
				fixture.candidate,
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("result-relation prompt omitted %q:\n%s", required, prompt)
				}
			}
			if strings.Count(prompt, fixture.candidate) != 1 ||
				strings.Contains(prompt, "APPLICATION REQUIREMENT INPUT") {
				t.Fatalf("result-relation prompt exceeded one-candidate authority:\n%s", prompt)
			}
			result, err := DecodeApplicationRequirementCandidateResultRelationResult(
				input, fixture.relation,
			)
			if err != nil || result.Relation != fixture.relation {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestApplicationRequirementCandidateResultRelationCorrectionIsBound(t *testing.T) {
	t.Parallel()
	request := "Build a browser route selector that scores available routes with the user-provided scoring rule and displays the highest-scoring route."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	authority := ApplicationIntentInput{UserRequest: request, Context: context}
	generation := applicationRequirementCandidateFixture(t, ApplicationRequirementCoverageInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
		AcceptedRequirements: []string{}, ExcludedCandidates: []string{},
	})
	current := "Accept values and display a correct result."
	candidateAuthority := applicationRequirementCandidateResultRelationInputFixture(t, current)
	relation, err := DecodeApplicationRequirementCandidateResultRelationResult(
		candidateAuthority,
		ApplicationRequirementMissingResultRelation,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationRequirementCandidateResultRelationCorrectionInput{
		GenerationAuthority: generation,
		CandidateAuthority:  candidateAuthority,
		ResultRelation:      relation,
	}
	prompt, err := BuildApplicationRequirementCandidateResultRelationCorrectionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		authority.UserRequest,
		current,
		ApplicationRequirementMissingResultRelation,
		"semantically entails exactly one determining rule",
	} {
		if strings.Count(prompt, required) != 1 {
			t.Fatalf("correction prompt did not bind %q exactly once:\n%s", required, prompt)
		}
	}
	corrected := "Score every available route with the user-provided scoring rule and display the highest-scoring route."
	if leaf, err := DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf(
		input, corrected,
	); err != nil || leaf != corrected {
		t.Fatalf("corrected=%q error=%v", leaf, err)
	}
	if _, err := DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf(
		input, current,
	); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("unchanged correction error=%v", err)
	}
	relation.Relation = ApplicationRequirementExplicitResultRelation
	input.ResultRelation = relation
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("non-defective relation opened a correction job")
	}
}

func TestApplicationRequirementCandidateResultRelationRejectsReceiptTamperingAndCorrectionDuplicates(
	t *testing.T,
) {
	t.Parallel()
	const current = "Accept values and display a correct result."
	authority := applicationRequirementCandidateResultRelationInputFixture(t, current)
	mutations := []func(*ApplicationRequirementCandidateResultRelationInput){
		func(input *ApplicationRequirementCandidateResultRelationInput) {
			input.Kind.Relation = ApplicationRequirementCandidateNonRuntime
		},
		func(input *ApplicationRequirementCandidateResultRelationInput) {
			input.Cardinality.CandidateSHA256 = strings.Repeat("0", 64)
		},
		func(input *ApplicationRequirementCandidateResultRelationInput) {
			input.Cardinality.Relation = ApplicationRequirementMultipleRuntimeOutcomes
		},
	}
	for index, mutate := range mutations {
		candidate := authority
		mutate(&candidate)
		if _, err := NewApplicationRequirementCandidateResultRelationJob(candidate); err == nil {
			t.Fatalf("tampered result-relation authority %d opened a job", index)
		}
	}

	request := "Build a browser label formatter that converts user-provided text to lowercase."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := "Convert the user-provided text to lowercase and display the formatted label."
	coverageInput := ApplicationRequirementCoverageInput{
		UserRequest: request, Context: context,
		AcceptedRequirements: []string{duplicate}, ExcludedCandidates: []string{},
	}
	generation := applicationRequirementCandidateFixture(t, coverageInput)
	relation, err := DecodeApplicationRequirementCandidateResultRelationResult(
		authority, ApplicationRequirementMissingResultRelation,
	)
	if err != nil {
		t.Fatal(err)
	}
	correction := ApplicationRequirementCandidateResultRelationCorrectionInput{
		GenerationAuthority: generation,
		CandidateAuthority:  authority,
		ResultRelation:      relation,
	}
	if _, err := DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf(
		correction, duplicate,
	); err == nil || !strings.Contains(err.Error(), "duplicated an accepted requirement") {
		t.Fatalf("duplicate correction error=%v", err)
	}
}

func TestApplicationRequirementCandidateResultRelationAcceptedReceiptIsTerminalAndBound(t *testing.T) {
	const candidate = "Show one public control."
	input := applicationRequirementCandidateResultRelationInputFixture(t, candidate)
	for _, relation := range []string{
		ApplicationRequirementNoDerivedResult,
		ApplicationRequirementExplicitResultRelation,
	} {
		result, err := DecodeApplicationRequirementCandidateResultRelationResult(input, relation)
		if err != nil {
			t.Fatal(err)
		}
		if err := result.ValidateAcceptedFor(candidate); err != nil {
			t.Fatalf("accepted relation %s was rejected: %v", relation, err)
		}
	}
	valid, err := DecodeApplicationRequirementCandidateResultRelationResult(
		input, ApplicationRequirementNoDerivedResult,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ApplicationRequirementCandidateResultRelationResult){
		"missing relation": func(result *ApplicationRequirementCandidateResultRelationResult) {
			result.Relation = ApplicationRequirementMissingResultRelation
		},
		"candidate drift": func(result *ApplicationRequirementCandidateResultRelationResult) {
			result.CandidateSHA256 = ExactObjectiveContextSHA("another candidate")
		},
		"kind receipt drift": func(result *ApplicationRequirementCandidateResultRelationResult) {
			result.KindReceiptSHA256 = strings.Repeat("0", 64)
		},
		"cardinality receipt drift": func(result *ApplicationRequirementCandidateResultRelationResult) {
			result.CardinalityReceiptSHA256 = strings.Repeat("0", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := valid
			mutate(&result)
			if err := result.ValidateAcceptedFor(candidate); err == nil {
				t.Fatalf("invalid accepted receipt was retained: %+v", result)
			}
		})
	}
}

func applicationRequirementCandidateResultRelationInputFixture(
	t testing.TB,
	candidate string,
) ApplicationRequirementCandidateResultRelationInput {
	t.Helper()
	kind, err := DecodeApplicationRequirementCandidateKindResult(
		ApplicationRequirementCandidateKindInput{Candidate: candidate},
		ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}
	cardinality, err := DecodeApplicationRequirementCandidateCardinalityResult(
		ApplicationRequirementCandidateCardinalityInput{Candidate: candidate},
		ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
}
