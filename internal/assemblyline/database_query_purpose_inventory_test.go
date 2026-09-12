package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestDatabasePurposeInventoryReturnsLinesOrExplicitAbsence(t *testing.T) {
	input := DatabaseQueryPurposeAuthority{
		State:      databaseParameterStateFixture("shipments", "weight", "dispatched_at"),
		Collection: DatabaseQueryFilterPurpose,
	}
	for _, candidates := range [][]string{{}, {"Require the package weight to exceed seven."}} {
		raw := strings.Join(candidates, "\n")
		if len(candidates) == 0 {
			raw = "NO_QUERY_PURPOSE_CANDIDATES"
		}
		inventory, err := DecodeDatabaseQueryPurposeInventory(input, raw)
		if err != nil || !reflect.DeepEqual(inventory.Candidates, candidates) {
			t.Fatalf("raw=%q inventory=%+v error=%v", raw, inventory, err)
		}
	}
	prompt, err := BuildDatabaseQueryPurposeInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, prompt, "NO_QUERY_PURPOSE_CANDIDATES", "one per line", input.State.Authority.ExactNeed)
	assertPromptOmitsPacketState(t, prompt, DatabaseQueryPurposeInventorySchemaV1, `"candidates"`, "Answer with A or B.")
}

func TestDatabasePurposeInventoryRejectsMalformedAndMixedAbsence(t *testing.T) {
	input := DatabaseQueryPurposeAuthority{
		State:      databaseParameterStateFixture("samples", "reading", "observed_at"),
		Collection: DatabaseQueryFilterPurpose,
	}
	const purpose = "Require the reading to exceed seven."
	for _, raw := range []string{
		"", "[]", `{ "candidates": [] }`, " NO_QUERY_PURPOSE_CANDIDATES", "NO_QUERY_PURPOSE_CANDIDATES ",
		"NO_QUERY_PURPOSE_CANDIDATES\n" + purpose, purpose + "\nNO_QUERY_PURPOSE_CANDIDATES",
		purpose + "\n\n" + purpose, purpose + "\r\n" + purpose,
		strings.Repeat(purpose+"\n", MaxDatabaseQueryPurposeCandidates) + purpose,
		strings.Repeat("x", maxDatabaseQueryPurposeBytes+1),
	} {
		inventory, err := DecodeDatabaseQueryPurposeInventory(input, raw)
		if err == nil || !reflect.DeepEqual(inventory, DatabaseQueryPurposeInventory{}) {
			t.Errorf("invalid raw=%q retained inventory=%+v error=%v", raw, inventory, err)
		}
	}
	for _, candidates := range [][]string{nil, {"NO_QUERY_PURPOSE_CANDIDATES"}} {
		inventory := DatabaseQueryPurposeInventory{Schema: DatabaseQueryPurposeInventorySchemaV1, Candidates: candidates}
		if err := inventory.ValidateFor(input); err == nil {
			t.Errorf("invalid retained inventory accepted: %+v", inventory)
		}
	}
}

func TestDatabasePurposePresenceWorkKindIsNotRegistered(t *testing.T) {
	kind := WorkKind("database_query_purpose_presence")
	if validWorkKind(kind) {
		t.Fatal("removed purpose-presence work kind remains registered")
	}
	job, err := newPortableJob(kind, DatabaseQueryPurposeAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Validate(); err == nil {
		t.Fatal("removed purpose-presence work still validates")
	}
	if _, err := RenderPortableJob(job); err == nil {
		t.Fatal("removed purpose-presence work still renders")
	}
	if _, err := SemanticUncertaintyContractForWorkKind(kind); err == nil {
		t.Fatal("removed purpose-presence work retains a semantic dispatch contract")
	}
	if _, err := PortableResponseFramingForWorkKind(kind); err == nil {
		t.Fatal("removed purpose-presence work retains provider framing")
	}
}
