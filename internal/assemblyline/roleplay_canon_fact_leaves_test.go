package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonFactLeavesReturnOneRawFactAtATime(t *testing.T) {
	source := RoleplayCanonSource{
		Kind:                  RoleplayCanonSourceUserContribution,
		AttributedPersonaName: "Bob",
		ExactContribution:     "Bob closed the west gate.",
		PersonaKind:           roleplay.UserPersonaCharacter,
		ContributionKind:      roleplay.UserContributionAction,
	}
	input := RoleplayCanonFactLeafInput{
		Source:        source,
		Context:       minifiedObjectiveContext("Bob is at the harbor."),
		AcceptedFacts: []string{},
	}
	job, err := NewRoleplayCanonFactCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, source.ExactContribution) ||
		!strings.Contains(prompt, RoleplayCanonFactRemains) {
		t.Fatalf("prompt=%q", prompt)
	}
	if _, err := DecodeRoleplayCanonFactCoverageLeaf(input, RoleplayCanonFactRemains); err != nil {
		t.Fatal(err)
	}
	fact, err := DecodeRoleplayCanonFactLeaf(input, "Bob closed the west gate.")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := AssembleRoleplayCanonExtractionDecision(
		input.extractionInput(), []string{fact},
	)
	if err != nil || len(decision.Facts) != 1 || decision.Facts[0] != fact {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestRoleplayCanonFactLeavesPreserveSourceBoundariesAndRejectStructuredOutput(t *testing.T) {
	input := RoleplayCanonFactLeafInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceUserContribution,
			AttributedPersonaName: "Mara",
			ExactContribution:     "Mara lowers the bridge.",
			PersonaKind:           roleplay.UserPersonaCharacter,
			ContributionKind:      roleplay.UserContributionAction,
		},
		Context:       minifiedObjectiveContext("The bridge is raised."),
		AcceptedFacts: []string{"Mara lowers the bridge."},
	}
	for _, raw := range []string{
		`{"facts":["Mara reaches the quay."]}`,
		`"Mara reaches the quay."`,
		"Mara lowers the bridge.",
	} {
		if _, err := DecodeRoleplayCanonFactLeaf(input, raw); err == nil {
			t.Fatalf("accepted invalid fact leaf %q", raw)
		}
	}
	input.Source.ExactContribution = strings.Repeat("x", roleplay.MaxUserTurnBytes+1)
	if _, err := NewRoleplayCanonFactCoverageJob(input); err == nil {
		t.Fatal("accepted oversized exact contribution")
	}
}
