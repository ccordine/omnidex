package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseQueryPurposeInventoryPreservesRawSourceOrder(t *testing.T) {
	t.Parallel()
	authority := DatabaseQueryPurposeAuthority{
		State:      readyDatabaseMaximumState(t, datasource.ResultRecords),
		Collection: DatabaseQueryFilterPurpose,
	}
	raw := "match the requested state\nignore archived records\nmatch the requested state"
	inventory, err := DecodeDatabaseQueryPurposeInventory(authority, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"match the requested state",
		"ignore archived records",
		"match the requested state",
	}
	if !reflect.DeepEqual(inventory.Candidates, want) {
		t.Fatalf("candidates=%v want=%v", inventory.Candidates, want)
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		t.Fatalf("raw hash=%q", inventory.RawSHA256)
	}
	if _, err := DecodeDatabaseQueryPurposeInventory(authority, `["match the requested state"]`); err == nil {
		t.Fatal("database query purpose inventory accepted JSON")
	}
}

func TestDatabaseQueryPurposeNecessityIsCandidateBoundAndNotCompletionAuthority(t *testing.T) {
	t.Parallel()
	authority := DatabaseQueryPurposeAuthority{
		State:      readyDatabaseMaximumState(t, datasource.ResultRecords),
		Collection: DatabaseQueryFilterPurpose,
	}
	inventory, err := DecodeDatabaseQueryPurposeInventory(
		authority, "match the requested state\napply a customary active-only filter",
	)
	if err != nil {
		t.Fatal(err)
	}
	input := DatabaseQueryPurposeNecessityInput{
		Authority: authority, Inventory: inventory, CandidateIndex: 1,
	}
	prompt, err := BuildDatabaseQueryPurposeNecessityPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, inventory.Candidates[1]) ||
		strings.Contains(prompt, inventory.Candidates[0]) {
		t.Fatalf("necessity prompt is not candidate-bound:\n%s", prompt)
	}
	result, err := DecodeDatabaseQueryPurposeNecessityResult(
		input, DatabaseQueryPurposeNotNecessary,
	)
	if err != nil || result.Relation != DatabaseQueryPurposeNotNecessary {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestDatabaseQueryPurposeRelationIsOnlyOnePairwiseIdentityQuestion(t *testing.T) {
	t.Parallel()
	input := DatabaseQueryPurposeRelationInput{
		Collection:      DatabaseQueryOrderPurpose,
		Candidate:       "sort the count from largest to smallest",
		AcceptedPurpose: "order by descending count",
	}
	prompt, err := BuildDatabaseQueryPurposeRelationPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"replace", "revoke", "complete the accepted", "workflow", "completion", "queue",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relation prompt exposed orchestration language %q:\n%s", forbidden, prompt)
		}
	}
	result, err := DecodeDatabaseQueryPurposeRelationResult(input, DatabaseQueryPurposesSame)
	if err != nil || result.Relation != DatabaseQueryPurposesSame {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestDatabaseQueryParameterPromptSeesAcceptedPurposeNotWholeEvidenceNeed(t *testing.T) {
	t.Parallel()
	state := readyDatabaseMaximumState(t, datasource.ResultRecords)
	const purpose = "match the explicitly requested state"
	input := DatabaseQueryFilterLeafInput{
		State: state, Purpose: purpose,
		AcceptedFilters: []datasource.RelationalPredicate{},
		AcceptedValues:  []datasource.IntentLiteral{},
	}
	prompt, err := BuildDatabaseQueryFilterFieldPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, purpose) {
		t.Fatalf("parameter prompt omitted accepted purpose:\n%s", prompt)
	}
	if strings.Contains(prompt, state.Authority.ExactNeed) {
		t.Fatalf("parameter prompt reopened the whole evidence need:\n%s", prompt)
	}
}
