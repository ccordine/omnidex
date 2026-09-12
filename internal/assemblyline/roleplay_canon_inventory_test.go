package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonInventoryReturnsFactsOrExplicitAbsence(t *testing.T) {
	input := roleplayCanonInventoryTestInput()
	for _, candidates := range [][]string{
		{},
		{"Mara locks the observatory door."},
		{"Mara locks the observatory door.", "Mara pockets the brass key."},
	} {
		raw := strings.Join(candidates, "\n")
		if len(candidates) == 0 {
			raw = "NO_CANON_FACT_CANDIDATES"
		}
		inventory, err := DecodeRoleplayCanonFactInventory(input, raw)
		if err != nil || !reflect.DeepEqual(inventory.Candidates, candidates) {
			t.Fatalf("raw=%q inventory=%+v error=%v", raw, inventory, err)
		}
	}
	prompt, err := BuildRoleplayCanonFactInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, prompt, "NO_CANON_FACT_CANDIDATES", "one fact per line", input.Source.ExactContribution)
	assertPromptOmitsPacketState(t, prompt, "Answer with A or B.", RoleplayCanonFactInventorySchemaV1, `"candidates"`, "source_kind")
}

func TestRoleplayCanonInventoryRejectsMalformedOrMixedAbsence(t *testing.T) {
	input := roleplayCanonInventoryTestInput()
	const fact = "Mara locks the observatory door."
	for _, raw := range []string{
		"", " ", "[]", `{ "candidates": [] }`,
		" NO_CANON_FACT_CANDIDATES", "NO_CANON_FACT_CANDIDATES ",
		"NO_CANON_FACT_CANDIDATES\n" + fact, fact + "\nNO_CANON_FACT_CANDIDATES",
		fact + "\r\n" + fact, fact + "\n\n" + fact,
		strings.Repeat(fact+"\n", MaxRoleplayCanonFactsPerTurn) + fact,
		strings.Repeat("x", roleplay.MaxCanonEventBytes+1),
	} {
		inventory, err := DecodeRoleplayCanonFactInventory(input, raw)
		if err == nil || !reflect.DeepEqual(inventory, RoleplayCanonFactInventory{}) {
			t.Errorf("invalid raw=%q retained inventory=%+v error=%v", raw, inventory, err)
		}
	}
	for _, candidates := range [][]string{nil, {"NO_CANON_FACT_CANDIDATES"}} {
		inventory := RoleplayCanonFactInventory{Schema: RoleplayCanonFactInventorySchemaV1, Candidates: candidates}
		if err := inventory.ValidateFor(input); err == nil {
			t.Errorf("invalid persisted inventory accepted: %+v", inventory)
		}
	}
}

func TestRoleplayCanonPresenceWorkKindIsNotRegistered(t *testing.T) {
	kind := WorkKind("roleplay_canon_fact_presence")
	if validWorkKind(kind) {
		t.Fatal("removed pre-inventory presence station remains registered")
	}
	job, err := newPortableJob(kind, roleplayCanonInventoryTestInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Validate(); err == nil {
		t.Fatal("removed presence station still validates as executable work")
	}
	if _, err := RenderPortableJob(job); err == nil {
		t.Fatal("removed presence station still renders model input")
	}
	if _, err := SemanticUncertaintyContractForWorkKind(kind); err == nil {
		t.Fatal("removed presence station retains a semantic dispatch contract")
	}
	if _, err := PortableResponseFramingForWorkKind(kind); err == nil {
		t.Fatal("removed presence station retains provider response framing")
	}
}

func roleplayCanonInventoryTestInput() RoleplayCanonExtractionInput {
	return RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceUserContribution,
			AttributedPersonaName: "Mara",
			ExactContribution:     "I lock the observatory door and pocket the brass key.",
			PersonaKind:           roleplay.UserPersonaCharacter,
			ContributionKind:      roleplay.UserContributionDialogue,
		},
		Context: ObjectiveContext{},
	}
}
