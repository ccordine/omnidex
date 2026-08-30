package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func roleplayCanonInventoryFixture() RoleplayCanonExtractionInput {
	return RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceUserContribution,
			AttributedPersonaName: "Bob",
			ExactContribution:     "Bob closed the west gate. The bronze bell cracked.",
			PersonaKind:           roleplay.UserPersonaCharacter,
			ContributionKind:      roleplay.UserContributionAction,
		},
		Context: minifiedObjectiveContext("Bob is at the harbor."),
	}
}

func TestRoleplayCanonFactInventoryIsOneUntrustedRawCandidateSet(t *testing.T) {
	input := roleplayCanonInventoryFixture()
	job, err := NewRoleplayCanonFactInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.Source.ExactContribution) ||
		!strings.Contains(prompt, "candidate durable fictional facts") ||
		strings.Contains(prompt, "accepted current-contribution") ||
		strings.Contains(prompt, "sieve") || strings.Contains(prompt, "workflow") {
		t.Fatalf("prompt=%q", prompt)
	}
	raw := "Bob closed the west gate.\nThe bronze bell cracked.\nBob closed the west gate."
	inventory, err := DecodeRoleplayCanonFactInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 3 || inventory.Candidates[2] != inventory.Candidates[0] {
		t.Fatalf("inventory=%+v", inventory)
	}
	if err := inventory.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	empty, err := DecodeRoleplayCanonFactInventory(input, RoleplayNoCanonFactCandidates)
	if err != nil || empty.Candidates == nil || len(empty.Candidates) != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	for _, invalid := range []string{`{"facts":["unsafe"]}`, "one\n\ntwo"} {
		if _, err := DecodeRoleplayCanonFactInventory(input, invalid); err == nil {
			t.Fatalf("accepted invalid inventory %q", invalid)
		}
	}
}

func TestRoleplayCanonFactCandidateSieveUsesNarrowRelations(t *testing.T) {
	input := roleplayCanonInventoryFixture()
	authorizationInput := RoleplayCanonFactCandidateAuthorizationInput{
		Authority: input,
		Candidate: "Bob closed the west gate.",
	}
	authorizationJob, err := NewRoleplayCanonFactCandidateAuthorizationJob(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(authorizationJob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, authorizationInput.Candidate) ||
		!strings.Contains(prompt, "one semantic entailment question") ||
		strings.Contains(prompt, "not canon generation") || strings.Contains(prompt, "sieve") {
		t.Fatalf("authorization prompt=%q", prompt)
	}
	authorization, err := DecodeRoleplayCanonFactCandidateAuthorization(
		authorizationInput, RoleplayCanonFactNotEstablished,
	)
	if err != nil || authorization.Relation != RoleplayCanonFactNotEstablished {
		t.Fatalf("authorization=%+v err=%v", authorization, err)
	}

	relationInput := RoleplayCanonFactCandidateRelationInput{
		Candidate:    "Bob shut the western gate.",
		AcceptedFact: "Bob closed the west gate.",
	}
	relation, err := DecodeRoleplayCanonFactCandidateRelation(
		relationInput, RoleplayCanonFactsEquivalent,
	)
	if err != nil || relation.Relation != RoleplayCanonFactsEquivalent {
		t.Fatalf("relation=%+v err=%v", relation, err)
	}
	relationInput.Candidate = relationInput.AcceptedFact
	if _, err := NewRoleplayCanonFactCandidateRelationJob(relationInput); err == nil {
		t.Fatal("exact duplicate reached the semantic relation station")
	}
}

func TestRoleplayCanonFactInventoryPreservesSourceBounds(t *testing.T) {
	input := roleplayCanonInventoryFixture()
	input.Source.ExactContribution = strings.Repeat("x", roleplay.MaxUserTurnBytes+1)
	if _, err := NewRoleplayCanonFactInventoryJob(input); err == nil {
		t.Fatal("accepted oversized exact contribution")
	}
}
