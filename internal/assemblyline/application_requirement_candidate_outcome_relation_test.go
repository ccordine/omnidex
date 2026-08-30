package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateOutcomeRelationIsOneBoundPair(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateOutcomeRelationFixture(
		t,
		"Render the normalized submitted label.",
		"Display the submitted label after normalization.",
	)
	job, err := NewApplicationRequirementCandidateOutcomeRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{input.Candidate, input.AcceptedRequirement} {
		if strings.Count(prompt, text) != 1 {
			t.Fatalf("outcome-relation prompt did not project one exact pair member %q:\n%s", text, prompt)
		}
	}
	for _, required := range []string{
		"same independently observable outcome",
		"distinct independently observable outcomes",
		"keeping it after a session restart is DISTINCT_RUNTIME_OUTCOMES",
		"returning it before a fixed deadline is DISTINCT_RUNTIME_OUTCOMES",
		"delivering it to a nonvisual consumer is DISTINCT_RUNTIME_OUTCOMES",
		"conforms to that same R is SAME_RUNTIME_OUTCOME",
		"Use this decision order after reading the exact pair",
		"exactly one statement adds runtime evidence",
		"Conformance of the identical value to its identical already-named determining rule is not added evidence",
		"A modifier alone does not add evidence",
		"no different delivery is required",
		"failing a later observation or retention check",
		"failing to deliver it through another channel",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("outcome-relation prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"user request", "accepted requirements", "workspace", "file path",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("outcome-relation prompt exposed %q:\n%s", forbidden, prompt)
		}
	}

	for _, relation := range []string{
		ApplicationRequirementSameRuntimeOutcome,
		ApplicationRequirementDistinctRuntimeOutcomes,
	} {
		result, err := DecodeApplicationRequirementCandidateOutcomeRelationResult(
			input, relation,
		)
		if err != nil || result.Relation != relation {
			t.Fatalf("relation=%q result=%+v error=%v", relation, result, err)
		}
		if err := result.ValidateFor(input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewApplicationRequirementCandidateOutcomeRelationJob(
		applicationRequirementCandidateOutcomeRelationFixture(t, input.Candidate, input.Candidate),
	); err == nil || !strings.Contains(err.Error(), "mechanically exact") {
		t.Fatalf("exact pair opened a model job: %v", err)
	}
}

func TestApplicationRequirementCandidateOutcomeRelationRejectsReceiptDrift(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateOutcomeRelationFixture(
		t,
		"Expose the selected record.",
		"Remove the selected record.",
	)
	valid, err := DecodeApplicationRequirementCandidateOutcomeRelationResult(
		input, ApplicationRequirementDistinctRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ApplicationRequirementCandidateOutcomeRelationResult){
		"candidate": func(value *ApplicationRequirementCandidateOutcomeRelationResult) {
			value.CandidateSHA256 = strings.Repeat("0", 64)
		},
		"accepted": func(value *ApplicationRequirementCandidateOutcomeRelationResult) {
			value.AcceptedRequirementSHA256 = strings.Repeat("0", 64)
		},
		"kind": func(value *ApplicationRequirementCandidateOutcomeRelationResult) {
			value.KindReceiptSHA256 = strings.Repeat("0", 64)
		},
		"cardinality": func(value *ApplicationRequirementCandidateOutcomeRelationResult) {
			value.CardinalityReceiptSHA256 = strings.Repeat("0", 64)
		},
		"accepted receipt": func(value *ApplicationRequirementCandidateOutcomeRelationResult) {
			value.AcceptedReceiptSHA256 = strings.Repeat("0", 64)
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := valid
			mutate(&changed)
			if err := changed.ValidateFor(input); err == nil {
				t.Fatal("drifted outcome-relation receipt was accepted")
			}
		})
	}
}

func applicationRequirementCandidateOutcomeRelationFixture(
	t testing.TB,
	candidate string,
	accepted string,
) ApplicationRequirementCandidateOutcomeRelationInput {
	t.Helper()
	candidateAuthority := applicationRequirementCandidateResultRelationInputFixture(t, candidate)
	acceptedAuthority := applicationRequirementCandidateResultRelationInputFixture(t, accepted)
	acceptedRelation, err := DecodeApplicationRequirementCandidateResultRelationResult(
		acceptedAuthority,
		ApplicationRequirementNoDerivedResult,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCandidateOutcomeRelationInput{
		Candidate:              candidate,
		Kind:                   candidateAuthority.Kind,
		Cardinality:            candidateAuthority.Cardinality,
		AcceptedRequirement:    accepted,
		AcceptedResultRelation: acceptedRelation,
	}
}
