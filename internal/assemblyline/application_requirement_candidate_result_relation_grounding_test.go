package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateResultRelationGroundingIsOneBoundQuestion(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a browser label normalizer that converts submitted text to Unicode lowercase."
	const candidate = "Transform submitted text and display the correct normalized label."
	input := applicationRequirementCandidateResultRelationGroundingInputFixture(
		t,
		request,
		candidate,
	)
	prompt, err := BuildApplicationRequirementCandidateResultRelationGroundingPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		request,
		candidate,
		ApplicationRequirementMissingResultRelation,
		"do the immutable request and established facts entail exactly one determining relation",
		"Do not propose a rule",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("grounding prompt omitted %q:\n%s", required, prompt)
		}
	}
	if strings.Count(prompt, request) != 1 || strings.Count(prompt, candidate) != 1 ||
		strings.Contains(prompt, "ACCEPTED REQUIREMENT") ||
		strings.Contains(prompt, "EXCLUDED") {
		t.Fatalf("grounding prompt exceeded request/candidate authority:\n%s", prompt)
	}
	for _, relation := range []string{
		ApplicationRequirementExactlyOneDeterminingRelationEntailed,
		ApplicationRequirementNoExactlyOneDeterminingRelationEntailed,
	} {
		result, err := DecodeApplicationRequirementCandidateResultRelationGroundingResult(
			input,
			relation,
		)
		if err != nil || result.Relation != relation {
			t.Fatalf("grounding=%+v error=%v", result, err)
		}
	}
}

func TestApplicationRequirementCandidateResultRelationGroundingRejectsAuthorityDrift(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a command that converts submitted text to Unicode uppercase."
	const candidate = "Display the correct converted text."
	input := applicationRequirementCandidateResultRelationGroundingInputFixture(
		t,
		request,
		candidate,
	)
	result, err := DecodeApplicationRequirementCandidateResultRelationGroundingResult(
		input,
		ApplicationRequirementExactlyOneDeterminingRelationEntailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ApplicationRequirementCandidateResultRelationGroundingResult){
		"request": func(value *ApplicationRequirementCandidateResultRelationGroundingResult) {
			value.ImmutableRequestSHA256 = ExactObjectiveContextSHA("another request")
		},
		"context": func(value *ApplicationRequirementCandidateResultRelationGroundingResult) {
			value.ApplicationContextSHA256 = strings.Repeat("0", 64)
		},
		"candidate": func(value *ApplicationRequirementCandidateResultRelationGroundingResult) {
			value.CandidateSHA256 = ExactObjectiveContextSHA("another candidate")
		},
		"missing receipt": func(value *ApplicationRequirementCandidateResultRelationGroundingResult) {
			value.MissingResultRelationReceiptSHA256 = strings.Repeat("0", 64)
		},
		"relation": func(value *ApplicationRequirementCandidateResultRelationGroundingResult) {
			value.Relation = "UNKNOWN"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidateResult := result
			mutate(&candidateResult)
			if err := candidateResult.ValidateFor(input); err == nil {
				t.Fatalf("drifted grounding receipt was accepted: %+v", candidateResult)
			}
		})
	}

	explicit := input
	explicit.MissingResultRelation, err = DecodeApplicationRequirementCandidateResultRelationResult(
		explicit.CandidateAuthority,
		ApplicationRequirementExplicitResultRelation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewApplicationRequirementCandidateResultRelationGroundingJob(explicit); err == nil {
		t.Fatal("an explicit result relation opened a grounding job")
	}
}

func TestApplicationRequirementCandidateResultRelationGroundingProjectsVerifiedContext(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a browser measurement converter using the established conversion policy."
	const candidate = "Accept a measurement and display the correct converted result."
	const fact = "The verified conversion policy multiplies the submitted yard value by 3 and reports feet."
	context := applicationRequirementResultRelationContextWithFact(t, request, fact)
	input := applicationRequirementCandidateResultRelationGroundingInputWithContextFixture(
		t,
		request,
		context,
		candidate,
	)
	prompt, err := BuildApplicationRequirementCandidateResultRelationGroundingPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, fact) || strings.Contains(prompt, "verified_relation_source") ||
		strings.Contains(prompt, "verified_relation_need") {
		t.Fatalf("grounding projection did not preserve only semantic context:\n%s", prompt)
	}
	result, err := DecodeApplicationRequirementCandidateResultRelationGroundingResult(
		input,
		ApplicationRequirementExactlyOneDeterminingRelationEntailed,
	)
	if err != nil || result.ApplicationContextSHA256 == "" {
		t.Fatalf("context-grounded result=%+v error=%v", result, err)
	}
}

func TestApplicationRequirementCandidateResultRelationGroundingRepresentsAbsentOrAmbiguousAuthority(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a browser measurement converter using a conversion policy."
	const candidate = "Accept a measurement and display the correct converted result."
	empty, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	ambiguous := applicationRequirementResultRelationContextWithFact(
		t,
		request,
		"The verified policy permits either a metric scale or an imperial scale and selects neither.",
	)
	for _, test := range []struct {
		name    string
		context ApplicationContext
	}{
		{name: "absent", context: empty},
		{name: "ambiguous", context: ambiguous},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := applicationRequirementCandidateResultRelationGroundingInputWithContextFixture(
				t,
				request,
				test.context,
				candidate,
			)
			result, err := DecodeApplicationRequirementCandidateResultRelationGroundingResult(
				input,
				ApplicationRequirementNoExactlyOneDeterminingRelationEntailed,
			)
			if err != nil || result.Relation != ApplicationRequirementNoExactlyOneDeterminingRelationEntailed {
				t.Fatalf("negative grounding=%+v error=%v", result, err)
			}
		})
	}
}

func applicationRequirementCandidateResultRelationGroundingInputFixture(
	t testing.TB,
	request string,
	candidate string,
) ApplicationRequirementCandidateResultRelationGroundingInput {
	t.Helper()
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	return applicationRequirementCandidateResultRelationGroundingInputWithContextFixture(
		t,
		request,
		context,
		candidate,
	)
}

func applicationRequirementCandidateResultRelationGroundingInputWithContextFixture(
	t testing.TB,
	request string,
	context ApplicationContext,
	candidate string,
) ApplicationRequirementCandidateResultRelationGroundingInput {
	t.Helper()
	candidateAuthority := applicationRequirementCandidateResultRelationInputFixture(t, candidate)
	missing, err := DecodeApplicationRequirementCandidateResultRelationResult(
		candidateAuthority,
		ApplicationRequirementMissingResultRelation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCandidateResultRelationGroundingInput{
		ImmutableRequest: request, Context: context, CandidateAuthority: candidateAuthority,
		MissingResultRelation: missing,
	}
}

func applicationRequirementResultRelationContextWithFact(
	t testing.TB,
	request string,
	factValue string,
) ApplicationContext {
	t.Helper()
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceExisting)
	if err != nil {
		t.Fatal(err)
	}
	context.Facts = append(context.Facts, ApplicationContextFact{
		ID: "fact_002", Kind: ApplicationContextRepositoryFact,
		Authority: ApplicationContextEvidenceAuthority, NeedID: "verified_relation_need",
		Value: factValue, SourceID: "verified_relation_source",
		SourceSHA256: ExactObjectiveContextSHA(factValue),
	})
	if err := context.Validate(); err != nil {
		t.Fatal(err)
	}
	return context
}
