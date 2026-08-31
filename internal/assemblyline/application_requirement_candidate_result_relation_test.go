package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateResultRelationUsesOnlyBoundBinaryQuestions(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name, candidate, relation string
		derived, determining      ApplicationRequirementCandidateResultPresence
	}{
		{
			name:        "ordered words",
			candidate:   "Sort the user-provided words in ascending Unicode code-point order and display the ordered words.",
			relation:    ApplicationRequirementExplicitResultRelation,
			derived:     ApplicationRequirementCandidateResultPresent,
			determining: ApplicationRequirementCandidateResultPresent,
		},
		{
			name:      "inventory status",
			candidate: "Display the current inventory status heading.",
			relation:  ApplicationRequirementNoDerivedResult,
			derived:   ApplicationRequirementCandidateResultAbsent,
		},
		{
			name:        "underdetermined recommendation",
			candidate:   "Accept a user's preferences and display the best recommendation.",
			relation:    ApplicationRequirementMissingResultRelation,
			derived:     ApplicationRequirementCandidateResultPresent,
			determining: ApplicationRequirementCandidateResultAbsent,
		},
		{
			name:        "action-form transformation",
			candidate:   "The finished software resizes supplied images.",
			relation:    ApplicationRequirementExplicitResultRelation,
			derived:     ApplicationRequirementCandidateResultPresent,
			determining: ApplicationRequirementCandidateResultPresent,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			finalInput := applicationRequirementCandidateResultRelationInputFixture(
				t, fixture.candidate,
			)
			derivedInput := ApplicationRequirementCandidateResultPresenceInput{
				Candidate: fixture.candidate, Kind: finalInput.Kind,
				Cardinality: finalInput.Cardinality,
				Dimension:   ApplicationRequirementDerivedValueDimension,
			}
			job, err := NewApplicationRequirementCandidateResultPresenceJob(derivedInput)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"does it assert a derived runtime value",
				"Return ABSENT when the candidate asserts only an action",
				"Is a derived runtime value PRESENT or ABSENT",
				fixture.candidate,
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("derived-value prompt omitted %q:\n%s", required, prompt)
				}
			}
			if strings.Count(prompt, fixture.candidate) != 1 ||
				strings.Contains(prompt, "independently computable determining relation") {
				t.Fatalf("derived-value prompt exceeded one binary question:\n%s", prompt)
			}
			derived, err := DecodeApplicationRequirementCandidateResultPresenceResult(
				derivedInput, string(fixture.derived),
			)
			if err != nil {
				t.Fatal(err)
			}

			var determining *ApplicationRequirementCandidateResultPresenceResult
			if fixture.derived == ApplicationRequirementCandidateResultPresent {
				determiningInput := ApplicationRequirementCandidateResultPresenceInput{
					Candidate: fixture.candidate, Kind: finalInput.Kind,
					Cardinality:          finalInput.Cardinality,
					Dimension:            ApplicationRequirementDeterminingRelationDimension,
					DerivedValuePresence: &derived,
				}
				determiningJob, err := NewApplicationRequirementCandidateResultPresenceJob(
					determiningInput,
				)
				if err != nil {
					t.Fatal(err)
				}
				determiningPrompt, err := RenderPortableJob(determiningJob)
				if err != nil {
					t.Fatal(err)
				}
				for _, required := range []string{
					"does it state an independently computable determining relation",
					"existing per-item grouping keys are rules",
					"actor-supplied expression, formula, or operation",
					"Is the independently computable determining relation PRESENT or ABSENT",
					fixture.candidate,
				} {
					if !strings.Contains(determiningPrompt, required) {
						t.Fatalf("determining-relation prompt omitted %q:\n%s", required, determiningPrompt)
					}
				}
				decoded, err := DecodeApplicationRequirementCandidateResultPresenceResult(
					determiningInput, string(fixture.determining),
				)
				if err != nil {
					t.Fatal(err)
				}
				determining = &decoded
			}
			result, err := ResolveApplicationRequirementCandidateResultRelation(
				finalInput, derived, determining,
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
		if _, err := NewApplicationRequirementCandidateResultPresenceJob(
			ApplicationRequirementCandidateResultPresenceInput{
				Candidate: candidate.Candidate, Kind: candidate.Kind,
				Cardinality: candidate.Cardinality,
				Dimension:   ApplicationRequirementDerivedValueDimension,
			},
		); err == nil {
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
		result, err := canonicalApplicationRequirementCandidateResultRelation(input, relation)
		if err != nil {
			t.Fatal(err)
		}
		if err := result.ValidateAcceptedFor(candidate); err != nil {
			t.Fatalf("accepted relation %s was rejected: %v", relation, err)
		}
	}
	valid, err := canonicalApplicationRequirementCandidateResultRelation(
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
	kind := applicationRequirementCandidateKindFixture(
		t,
		candidate,
		ApplicationRequirementCandidateTaskLocal,
	)
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
