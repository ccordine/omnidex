package queue

import (
	"os"
	"strings"
	"testing"
)

const sourceDeclarationProjectionMigration = "161_source_declaration_projection_authority.sql"

func TestSourceDeclarationProjectionMigrationRegistersExactLanguageBoundSpan(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + sourceDeclarationProjectionMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"'exact_response','source_declaration','typescript_function'",
		"gap_work_kind='fragment_correction'",
		"gap_work_kind='fragment_generation'",
		"gap_work_kind='fragment_modification'",
		"gap_payload->>'language' IN ('go','javascript','java','rust','php')",
		"gap_payload->>'language'='go'",
		"7107b5b1702f19fc23bd2d8d196edeb40c48132a921d22b373d64ca5c29e6409",
		"1738890c05fe810cf0978011e6e512f7a132e8622b1eaee3cedda4cd7618b163",
		"substring(convert_to(call_response,'UTF8') FROM NEW.source_start_byte+1",
		"IS DISTINCT FROM convert_to(NEW.response,'UTF8')",
		") NOT VALID",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("source declaration projection migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_outcomes", "fragment_correction') AND\n            gap_payload->>'language' IN",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("source declaration projection migration contains %q", forbidden)
		}
	}
}

func TestSourceDeclarationProjectionMigrationEnforcesExactReceiptSpan(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, legacyStationOutputProjectionSchema); err != nil {
		t.Fatal(err)
	}
	prior, err := os.ReadFile("../../migrations/095_station_output_artifact_projection.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(prior)); err != nil {
		t.Fatalf("apply prior projection migration: %v", err)
	}
	current, err := os.ReadFile("../../migrations/" + sourceDeclarationProjectionMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(current)); err != nil {
		t.Fatalf("apply source declaration projection migration: %v", err)
	}

	rawResponse := "```go\nfunc Value() int { return 1 }\n```"
	declaration := "func Value() int { return 1 }"
	startByte := strings.Index(rawResponse, declaration)
	insertSourceDeclarationProjectionAuthority(
		t, pool, 10, "fragment_generation", "go", rawResponse,
	)
	if err := insertSourceDeclarationProjectionOutcome(
		t, pool, 10, rawResponse, declaration, startByte,
	); err != nil {
		t.Fatalf("insert exact source declaration projection: %v", err)
	}

	insertSourceDeclarationProjectionAuthority(
		t, pool, 11, "fragment_generation", "typescript", rawResponse,
	)
	if err := insertSourceDeclarationProjectionOutcome(
		t, pool, 11, rawResponse, declaration, startByte,
	); err == nil ||
		!strings.Contains(err.Error(), "projection differs") {
		t.Fatalf("unregistered language used source declaration projection: %v", err)
	}

	insertSourceDeclarationProjectionAuthority(
		t, pool, 12, "fragment_modification", "javascript", rawResponse,
	)
	if err := insertSourceDeclarationProjectionOutcome(
		t, pool, 12, rawResponse, declaration, startByte,
	); err == nil || !strings.Contains(err.Error(), "projection differs") {
		t.Fatalf("unsupported modification used source declaration projection: %v", err)
	}

	insertSourceDeclarationProjectionAuthority(
		t, pool, 13, "fragment_correction", "", rawResponse,
	)
	if err := insertSourceDeclarationProjectionOutcome(
		t, pool, 13, rawResponse, declaration, startByte,
	); err != nil {
		t.Fatalf("fragment correction exact span was rejected: %v", err)
	}
}
