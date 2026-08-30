package assemblyline

import (
	"encoding/json"
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
				"Classify one fact about this exact one-outcome runtime requirement",
				"Apply these steps in order",
				"Detect a derived value when the candidate requires an observable value selected, ordered, transformed, hashed, grouped, aggregated, measured, calculated, or decided",
				"Data described as converted or transformed is not supplied data unchanged",
				"existing per-item grouping key is a stated rule",
				"equal observed key values determine groups",
				"Only when no derived value is asserted",
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
			first := strings.Index(prompt, "1. Detect a derived value")
			second := strings.Index(prompt, "2. For a derived value")
			third := strings.Index(prompt, "3. Only when no derived value")
			if first < 0 || second <= first || third <= second {
				t.Fatalf("result-relation rules are not ordered:\n%s", prompt)
			}
			if overhead := len(strings.Fields(prompt)) - len(strings.Fields(fixture.candidate)); overhead > 220 {
				t.Fatalf("result-relation prompt overhead=%d words", overhead)
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
	current := "Accept values and display a correct result."
	groundingInput := applicationRequirementCandidateResultRelationGroundingInputFixture(
		t,
		request,
		current,
	)
	grounding, err := DecodeApplicationRequirementCandidateResultRelationGroundingResult(
		groundingInput,
		ApplicationRequirementExactlyOneDeterminingRelationEntailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationRequirementCandidateResultRelationCorrectionInput{
		ImmutableRequest: request,
		Context:          groundingInput.Context,
		CurrentCandidate: current,
		Defect:           ApplicationRequirementMissingResultRelation,
		Grounding:        grounding,
	}
	prompt, err := BuildApplicationRequirementCandidateResultRelationCorrectionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		request,
		current,
		ApplicationRequirementMissingResultRelation,
	} {
		if strings.Count(prompt, required) != 1 {
			t.Fatalf("correction prompt did not bind %q exactly once:\n%s", required, prompt)
		}
	}
	job, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(job.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope) != 5 || envelope["immutable_request"] == nil ||
		envelope["context"] == nil ||
		envelope["current_candidate"] == nil || envelope["defect"] == nil ||
		envelope["grounding"] == nil {
		t.Fatalf("correction envelope contains non-minimal authority: %s", job.Payload)
	}
	for _, forbidden := range []string{
		"accepted_requirements", "excluded_candidates", "generation_authority",
	} {
		if strings.Contains(prompt, forbidden) || strings.Contains(string(job.Payload), forbidden) {
			t.Fatalf("correction envelope leaked workflow collection %q", forbidden)
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
	input.Defect = ApplicationRequirementExplicitResultRelation
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("non-defective relation opened a correction job")
	}
	input.Defect = ApplicationRequirementMissingResultRelation
	input.Grounding, err = DecodeApplicationRequirementCandidateResultRelationGroundingResult(
		groundingInput,
		ApplicationRequirementNoExactlyOneDeterminingRelationEntailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("negative grounding opened a correction job")
	}
	input.Grounding = ApplicationRequirementCandidateResultRelationGroundingResult{}
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("absent grounding opened a correction job")
	}
	input.Grounding = grounding
	input.Grounding.CandidateSHA256 = ExactObjectiveContextSHA("another candidate")
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("candidate-drifted grounding opened a correction job")
	}
	input.Grounding = grounding
	input.Context.Facts[0].SourceID = "different_workspace_authority"
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("context-drifted grounding opened a correction job")
	}
	input.Context.Facts[0].SourceID = "workspace"
	input.Context = groundingInput.Context
	input.ImmutableRequest += " Add another behavior."
	if _, err := NewApplicationRequirementCandidateResultRelationCorrectionJob(input); err == nil {
		t.Fatal("request-drifted grounding opened a correction job")
	}
}

func TestApplicationRequirementCandidateResultRelationRejectsReceiptTampering(
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
