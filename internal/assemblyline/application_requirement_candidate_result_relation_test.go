package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementDeterminingRelationPromptDefinesParametricRules(t *testing.T) {
	t.Parallel()
	for _, candidate := range []string{
		"The finished software performs unit-conversion operations on supplied measurements.",
		"The finished software performs statistical aggregation operations on supplied observations.",
		"The finished software reports the dimensions of each transformed image.",
		"The finished software reports the item count of each supplied batch.",
	} {
		candidate := candidate
		t.Run(candidate, func(t *testing.T) {
			t.Parallel()
			authority := applicationRequirementCandidateResultRelationAuthorityFixture(t, candidate)
			derivedInput := ApplicationRequirementCandidateResultPresenceInput{
				Candidate: candidate, Kind: authority.Kind, Cardinality: authority.Cardinality,
				Dimension: ApplicationRequirementDerivedValueDimension,
			}
			derived, err := DecodeApplicationRequirementCandidateResultPresenceResult(
				derivedInput, string(ApplicationRequirementCandidateResultPresent),
			)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := BuildApplicationRequirementCandidateResultPresencePrompt(
				ApplicationRequirementCandidateResultPresenceInput{
					Candidate: candidate, Kind: authority.Kind, Cardinality: authority.Cardinality,
					Dimension:            ApplicationRequirementDeterminingRelationDimension,
					DerivedValuePresence: &derived,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				candidate,
				"family of result-bearing operations over governed inputs",
				"The family name and governed inputs are sufficient by themselves",
				"named intrinsic or mechanically observable property",
				"Named dimensions, lengths, counts",
				"Do not apply this ABSENT rule to a named operation family over governed inputs or a named intrinsic property",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("prompt does not contain required operation-family contract %q", required)
				}
			}
		})
	}
}

func TestApplicationRequirementCandidateResultRelationFoldsBoundBinaryLeaves(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name          string
		candidate     string
		derived       ApplicationRequirementCandidateResultPresence
		determining   ApplicationRequirementCandidateResultPresence
		wantRelation  string
		wantCallCount int
	}{
		{
			name:          "ordered text",
			candidate:     "The finished software orders supplied words by ascending Unicode code point.",
			derived:       ApplicationRequirementCandidateResultPresent,
			determining:   ApplicationRequirementCandidateResultPresent,
			wantRelation:  ApplicationRequirementExplicitResultRelation,
			wantCallCount: 2,
		},
		{
			name:          "record digest",
			candidate:     "The finished software computes the SHA-256 digest of the supplied record bytes.",
			derived:       ApplicationRequirementCandidateResultPresent,
			determining:   ApplicationRequirementCandidateResultPresent,
			wantRelation:  ApplicationRequirementExplicitResultRelation,
			wantCallCount: 2,
		},
		{
			name:          "status heading",
			candidate:     "The finished software displays the current inventory status heading.",
			derived:       ApplicationRequirementCandidateResultAbsent,
			wantRelation:  ApplicationRequirementNoDerivedResult,
			wantCallCount: 1,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			authority := applicationRequirementCandidateResultRelationAuthorityFixture(
				t, fixture.candidate,
			)
			derivedInput := ApplicationRequirementCandidateResultPresenceInput{
				Candidate: authority.Candidate, Kind: authority.Kind,
				Cardinality: authority.Cardinality,
				Dimension:   ApplicationRequirementDerivedValueDimension,
			}
			derived, err := DecodeApplicationRequirementCandidateResultPresenceResult(
				derivedInput, string(fixture.derived),
			)
			if err != nil {
				t.Fatal(err)
			}
			calls := 1
			var determining *ApplicationRequirementCandidateResultPresenceResult
			if fixture.derived == ApplicationRequirementCandidateResultPresent {
				determiningInput := ApplicationRequirementCandidateResultPresenceInput{
					Candidate: authority.Candidate, Kind: authority.Kind,
					Cardinality:          authority.Cardinality,
					Dimension:            ApplicationRequirementDeterminingRelationDimension,
					DerivedValuePresence: &derived,
				}
				decoded, decodeErr := DecodeApplicationRequirementCandidateResultPresenceResult(
					determiningInput, string(fixture.determining),
				)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				determining = &decoded
				calls++
			}
			result, err := ResolveApplicationRequirementCandidateResultRelation(
				authority, derived, determining,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Relation != fixture.wantRelation || calls != fixture.wantCallCount {
				t.Fatalf("relation=%q calls=%d", result.Relation, calls)
			}
		})
	}
}

func TestApplicationRequirementCandidateResultRelationRejectsTamperAndUnderdeterminedReceipt(
	t *testing.T,
) {
	t.Parallel()
	const candidate = "The finished software selects an appropriate destination for supplied material."
	authority := applicationRequirementCandidateResultRelationAuthorityFixture(t, candidate)
	derivedInput := ApplicationRequirementCandidateResultPresenceInput{
		Candidate: candidate, Kind: authority.Kind, Cardinality: authority.Cardinality,
		Dimension: ApplicationRequirementDerivedValueDimension,
	}
	derived, err := DecodeApplicationRequirementCandidateResultPresenceResult(
		derivedInput, string(ApplicationRequirementCandidateResultPresent),
	)
	if err != nil {
		t.Fatal(err)
	}
	determiningInput := ApplicationRequirementCandidateResultPresenceInput{
		Candidate: candidate, Kind: authority.Kind, Cardinality: authority.Cardinality,
		Dimension:            ApplicationRequirementDeterminingRelationDimension,
		DerivedValuePresence: &derived,
	}
	determining, err := DecodeApplicationRequirementCandidateResultPresenceResult(
		determiningInput, string(ApplicationRequirementCandidateResultAbsent),
	)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := ResolveApplicationRequirementCandidateResultRelation(
		authority, derived, &determining,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.ValidateAcceptedFor(candidate); err == nil || !strings.Contains(err.Error(), "cannot be retained") {
		t.Fatalf("under-determined receipt was retained: %v", err)
	}

	tampered := authority
	tampered.Kind.CandidateSHA256 = strings.Repeat("0", 64)
	if _, err := NewApplicationRequirementCandidateResultPresenceJob(
		ApplicationRequirementCandidateResultPresenceInput{
			Candidate: candidate, Kind: tampered.Kind, Cardinality: tampered.Cardinality,
			Dimension: ApplicationRequirementDerivedValueDimension,
		},
	); err == nil {
		t.Fatal("candidate-drifted kind receipt opened a result-relation question")
	}
	tampered = authority
	tampered.Cardinality.Relation = ApplicationRequirementMultipleRuntimeOutcomes
	if _, err := NewApplicationRequirementCandidateResultPresenceJob(
		ApplicationRequirementCandidateResultPresenceInput{
			Candidate: candidate, Kind: tampered.Kind, Cardinality: tampered.Cardinality,
			Dimension: ApplicationRequirementDerivedValueDimension,
		},
	); err == nil {
		t.Fatal("multi-outcome cardinality receipt opened a result-relation question")
	}
}

func TestApplicationRequirementOutcomeRelationBindsAcceptedResultReceipt(t *testing.T) {
	t.Parallel()
	const current = "The finished software shows a current records heading."
	const accepted = "The finished software shows an active records heading."
	currentAuthority := applicationRequirementCandidateResultRelationAuthorityFixture(t, current)
	acceptedAuthority := applicationRequirementCandidateResultRelationAuthorityFixture(t, accepted)
	derivedInput := ApplicationRequirementCandidateResultPresenceInput{
		Candidate: accepted, Kind: acceptedAuthority.Kind,
		Cardinality: acceptedAuthority.Cardinality,
		Dimension:   ApplicationRequirementDerivedValueDimension,
	}
	derived, err := DecodeApplicationRequirementCandidateResultPresenceResult(
		derivedInput, string(ApplicationRequirementCandidateResultAbsent),
	)
	if err != nil {
		t.Fatal(err)
	}
	acceptedReceipt, err := ResolveApplicationRequirementCandidateResultRelation(
		acceptedAuthority, derived, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationRequirementCandidateOutcomeRelationInput{
		Candidate: current, Kind: currentAuthority.Kind,
		Cardinality:         currentAuthority.Cardinality,
		AcceptedRequirement: accepted, AcceptedResultRelation: acceptedReceipt,
	}
	result, err := DecodeApplicationRequirementCandidateOutcomeRelationResult(
		input, ApplicationRequirementDistinctRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	result.AcceptedReceiptSHA256 = strings.Repeat("f", 64)
	if err := result.ValidateFor(input); err == nil {
		t.Fatal("outcome relation accepted a drifted accepted-receipt hash")
	}
	input.AcceptedResultRelation.CandidateSHA256 = strings.Repeat("e", 64)
	if _, err := NewApplicationRequirementCandidateOutcomeRelationJob(input); err == nil {
		t.Fatal("outcome relation opened with a candidate-drifted accepted receipt")
	}
}

func applicationRequirementCandidateResultRelationAuthorityFixture(
	t testing.TB,
	candidate string,
) ApplicationRequirementCandidateResultRelationInput {
	t.Helper()
	runtimeInput := ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate, Dimension: ApplicationRequirementCandidateRuntimeContentDimension,
	}
	runtimeContent, err := DecodeApplicationRequirementCandidateContentPresenceResult(
		runtimeInput, string(ApplicationRequirementCandidateContentPresent),
	)
	if err != nil {
		t.Fatal(err)
	}
	nonRuntimeInput := ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate, Dimension: ApplicationRequirementCandidateNonRuntimeContentDimension,
	}
	nonRuntimeContent, err := DecodeApplicationRequirementCandidateContentPresenceResult(
		nonRuntimeInput, string(ApplicationRequirementCandidateContentAbsent),
	)
	if err != nil {
		t.Fatal(err)
	}
	kind, resolved, err := ResolveApplicationRequirementCandidateKind(
		candidate, runtimeContent, nonRuntimeContent,
	)
	if err != nil || !resolved {
		t.Fatalf("kind=%+v resolved=%t error=%v", kind, resolved, err)
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
