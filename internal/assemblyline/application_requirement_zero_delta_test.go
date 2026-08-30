package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementExactZeroDeltaReentersChangedCoverageAuthority(t *testing.T) {
	t.Parallel()
	authority := applicationRequirementZeroDeltaAuthority(t)
	current := applicationRequirementCandidateFixture(t, authority)
	before, err := NewApplicationRequirementCoverageJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	evidence := ApplicationRequirementCandidateZeroDelta{
		Candidate:     authority.AcceptedRequirements[0],
		RetainedSet:   ApplicationRequirementZeroDeltaAcceptedSet,
		RetainedIndex: 0,
	}
	rebound, err := RecordApplicationRequirementCandidateZeroDelta(current, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebound.ZeroDeltas) != 1 || rebound.ZeroDeltas[0] != evidence {
		t.Fatalf("rebound zero deltas=%+v", rebound.ZeroDeltas)
	}
	after, err := NewApplicationRequirementCoverageJob(rebound)
	if err != nil {
		t.Fatal(err)
	}
	if before.ID == after.ID {
		t.Fatal("zero delta did not change ordinary coverage authority")
	}
	prompt, err := RenderPortableJob(after)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, evidence.Candidate) ||
		!strings.Contains(prompt, "CODE-ESTABLISHED ZERO-DELTA") {
		t.Fatalf("coverage prompt omitted zero-delta evidence:\n%s", prompt)
	}

	reboundCoverage, err := DecodeApplicationRequirementCoverageLeaf(
		rebound, ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RecordApplicationRequirementCandidateZeroDelta(
		ApplicationRequirementCandidateInput{Authority: rebound, Coverage: reboundCoverage},
		evidence,
	)
	if err == nil || !strings.Contains(err.Error(), "already recorded") {
		t.Fatalf("repeated zero delta error=%v", err)
	}
}

func TestApplicationRequirementSemanticZeroDeltaRequiresSameOutcomeReceipt(t *testing.T) {
	t.Parallel()
	authority := applicationRequirementZeroDeltaAuthority(t)
	candidate := "Show the submitted label after trimming its surrounding spacing."
	relationInput := applicationRequirementCandidateOutcomeRelationFixture(
		t, candidate, authority.AcceptedRequirements[0],
	)
	same, err := DecodeApplicationRequirementCandidateOutcomeRelationResult(
		relationInput, ApplicationRequirementSameRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence := ApplicationRequirementCandidateZeroDelta{
		Candidate:              candidate,
		RetainedSet:            ApplicationRequirementZeroDeltaAcceptedSet,
		RetainedIndex:          0,
		CandidateKind:          relationInput.Kind,
		CandidateCardinality:   relationInput.Cardinality,
		AcceptedResultRelation: relationInput.AcceptedResultRelation,
		OutcomeRelation:        same,
	}
	if _, err := RecordApplicationRequirementCandidateZeroDelta(
		applicationRequirementCandidateFixture(t, authority), evidence,
	); err != nil {
		t.Fatal(err)
	}

	distinct, err := DecodeApplicationRequirementCandidateOutcomeRelationResult(
		relationInput, ApplicationRequirementDistinctRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence.OutcomeRelation = distinct
	if _, err := RecordApplicationRequirementCandidateZeroDelta(
		applicationRequirementCandidateFixture(t, authority), evidence,
	); err == nil || !strings.Contains(err.Error(), "requires relation") {
		t.Fatalf("distinct outcome was recorded as zero delta: %v", err)
	}
}

func TestApplicationRequirementZeroDeltaRequiresMechanicalIdentityPrecedence(t *testing.T) {
	t.Parallel()
	authority := applicationRequirementZeroDeltaAuthority(t)
	authority.AcceptedRequirements = append(
		authority.AcceptedRequirements,
		"Expose a reset control.",
	)
	authority.ExcludedCandidates = append(
		authority.ExcludedCandidates,
		"Use a particular implementation library.",
	)
	current := applicationRequirementCandidateFixture(t, authority)

	t.Run("wrong accepted index", func(t *testing.T) {
		evidence := ApplicationRequirementCandidateZeroDelta{
			Candidate:     authority.AcceptedRequirements[0],
			RetainedSet:   ApplicationRequirementZeroDeltaAcceptedSet,
			RetainedIndex: 1,
		}
		if _, err := RecordApplicationRequirementCandidateZeroDelta(
			current, evidence,
		); err == nil || !strings.Contains(err.Error(), "requires retained identity") {
			t.Fatalf("wrong exact accepted identity error=%v", err)
		}
	})

	t.Run("excluded candidate claimed as semantic accepted", func(t *testing.T) {
		candidate := authority.ExcludedCandidates[0]
		relationInput := applicationRequirementCandidateOutcomeRelationFixture(
			t, candidate, authority.AcceptedRequirements[0],
		)
		same, err := DecodeApplicationRequirementCandidateOutcomeRelationResult(
			relationInput, ApplicationRequirementSameRuntimeOutcome,
		)
		if err != nil {
			t.Fatal(err)
		}
		evidence := ApplicationRequirementCandidateZeroDelta{
			Candidate:              candidate,
			RetainedSet:            ApplicationRequirementZeroDeltaAcceptedSet,
			RetainedIndex:          0,
			CandidateKind:          relationInput.Kind,
			CandidateCardinality:   relationInput.Cardinality,
			AcceptedResultRelation: relationInput.AcceptedResultRelation,
			OutcomeRelation:        same,
		}
		if _, err := RecordApplicationRequirementCandidateZeroDelta(
			current, evidence,
		); err == nil || !strings.Contains(err.Error(), "requires retained identity") {
			t.Fatalf("excluded exact identity accepted as semantic evidence: %v", err)
		}
	})
}

func applicationRequirementZeroDeltaAuthority(
	t testing.TB,
) ApplicationRequirementCoverageInput {
	t.Helper()
	request := "Build a browser label cleaner that trims submitted labels and exposes a reset control."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCoverageInput{
		UserRequest:          request,
		Context:              context,
		AcceptedRequirements: []string{"Display the submitted label without surrounding spacing."},
		ExcludedCandidates:   []string{},
		ZeroDeltas:           []ApplicationRequirementCandidateZeroDelta{},
	}
}
