package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestContextMinificationAcceptsMarkdownAsOrdinarySemanticText(t *testing.T) {
	authority, err := NewContextCandidateAuthority(
		"session_turn", "CTX_1", "The current status is ready.",
	)
	if err != nil {
		t.Fatalf("context authority: %v", err)
	}
	input := ContextMinificationInput{
		ExactInstruction:    "Summarize the relevant current state.",
		SelectedAuthorities: []ContextCandidateAuthority{authority},
		KnownArtifactPaths:  []string{},
	}
	const raw = "## Current state\n\n```json\n{\"status\":\"ready\"}\n```"
	decision, err := DecodeContextMinificationDecision(input, raw)
	if err != nil {
		t.Fatalf("decode Markdown context: %v", err)
	}
	if decision.MinimalContext != raw {
		t.Fatalf("context text changed: got %q want %q", decision.MinimalContext, raw)
	}
}

func TestApplicationProductContextAcceptsJSONShapedOrdinaryText(t *testing.T) {
	const request = "Build scheduling software for a repair shop."
	context, err := BootstrapApplicationContext(request)
	if err != nil {
		t.Fatalf("bootstrap application context: %v", err)
	}
	input := ApplicationProductContextInput{UserRequest: request, Context: context}
	const raw = `{"domain":"repair-shop scheduling"}`
	value, err := DecodeApplicationProductContextLeaf(input, raw)
	if err != nil {
		t.Fatalf("decode JSON-shaped product context: %v", err)
	}
	if value != raw {
		t.Fatalf("product context changed: got %q want %q", value, raw)
	}
}

func TestOpenDatabaseTextLiteralAcceptsJSONShapedOrdinaryText(t *testing.T) {
	input := openTextDatabaseFilterInput()
	const raw = `{"status":"ready"}`
	literal, err := DecodeDatabaseQueryFilterValueLeaf(input, raw)
	if err != nil {
		t.Fatalf("decode JSON-shaped text literal: %v", err)
	}
	if literal.Type != datasource.LiteralString || literal.Value != raw {
		t.Fatalf("literal = %#v, want exact string %q", literal, raw)
	}
}

func TestCanonicalAndInventoryLeavesRemainStrict(t *testing.T) {
	choices := make([]OpaqueModelChoice, 0, 2)
	for _, spec := range [][2]string{{"first", "internal-first"}, {"second", "internal-second"}} {
		choice, err := NewOpaqueModelChoice(spec[0], spec[1])
		if err != nil {
			t.Fatalf("choice: %v", err)
		}
		choices = append(choices, choice)
	}
	if _, err := DecodeOpaqueModelChoice(`{"choice":"A"}`, choices); err == nil {
		t.Fatal("opaque selection accepted a JSON response packet")
	}

	const request = "Build a repair scheduling tool."
	context, err := BootstrapApplicationContext(request)
	if err != nil {
		t.Fatalf("bootstrap application context: %v", err)
	}
	inventoryInput := ApplicationRequirementInventoryInput{
		UserRequest: request,
		Context:     context,
	}
	if _, err := DecodeApplicationRequirementInventory(
		inventoryInput, `{"requirements":["Schedule repairs"]}`,
	); err == nil {
		t.Fatal("requirement inventory accepted a JSON response packet")
	}
}

func openTextDatabaseFilterInput() DatabaseQueryFilterLeafInput {
	const fingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	authority := DatabaseQueryIntentInput{
		EvidenceNeedID: "need_1",
		ExactNeed:      "Find records whose payload exactly matches the requested text.",
		Context:        ObjectiveContext{},
		SchemaProjection: datasource.IntentSchemaProjection{
			Schema: datasource.IntentSchemaProjectionV1, SourceID: "source_1",
			SchemaFingerprint: fingerprint,
			Relations: []datasource.IntentRelationProjection{{
				ID: "relation_1", SchemaName: "public", Name: "events",
				Kind: datasource.RelationTable,
				Columns: []datasource.IntentColumnProjection{{
					ID: "column_1", Name: "payload", TypeCategory: datasource.TypeText,
				}},
				PrimaryKey:  []string{},
				ForeignKeys: []datasource.IntentForeignKeyProjection{},
			}},
		},
		TemporalAsOf: "2026-09-01T12:00:00Z",
		MaxRows:      10,
	}
	state := NewDatabaseQueryIntentLeafState(authority)
	state.FromRelationID = "relation_1"
	state.Shape = datasource.ResultRecords
	return DatabaseQueryFilterLeafInput{
		State: state, Purpose: "Match the requested payload.",
		AcceptedFilters: []datasource.RelationalPredicate{},
		FieldID:         "column_1", Operator: datasource.FilterEqual,
		AcceptedValues: []datasource.IntentLiteral{},
	}
}

func TestOrdinarySemanticTextStillEnforcesTransportBounds(t *testing.T) {
	if _, err := decodeOrdinarySemanticText("ordinary text", strings.Repeat("x", 5), 4); err == nil {
		t.Fatal("ordinary semantic text exceeded its station byte bound")
	}
	if _, err := decodeOrdinarySemanticText("ordinary text", "value\x00tail", 32); err == nil {
		t.Fatal("ordinary semantic text accepted NUL")
	}
}
