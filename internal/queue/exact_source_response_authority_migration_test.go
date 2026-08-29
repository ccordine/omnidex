package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const exactSourceResponseAuthorityMigration = "178_exact_source_response_authority.sql"

func TestExactSourceResponseAuthorityMigrationRegistersOneFullResponse(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + exactSourceResponseAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"97524a146f254b2390f59e4581994ad21c9b16702123f7340b7dad1c681de37b",
		"ab90a5c3cb073ed1440e4d837d8eb270825c846fbf6c3f78bb7b8c6d40baed53",
		"cc9f4f06c00cab22eb2190eb04d8e5fa7c210bcfef0a9d98937b525f2e969914",
		"6aed5364ecd08519d089cba8ec1923a96fcc23ecdb5396a54cdd043546cc74e8",
		"source_response_sha256=response_sha256",
		"source_start_byte=0 AND source_end_byte=octet_length(response)",
		"NEW.source_start_byte<>0",
		"NEW.source_end_byte<>octet_length(call_response)",
		"NEW.response IS DISTINCT FROM call_response",
		"VALIDATE CONSTRAINT station_gap_outcomes_exact_source_response",
		"requires a fresh reset",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("exact source response migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_outcomes",
		"DROP CONSTRAINT station_gap_outcomes_projected_response",
		"substring(convert_to(call_response",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("exact source response migration contains %q", forbidden)
		}
	}
}

func TestExactSourceResponseAuthorityMigrationPreservesHistoricalUnprojectedOutcome(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	if _, err := pool.Exec(t.Context(), legacyStationOutputProjectionSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), legacyStationOutputProjectionFixture); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"095_station_output_artifact_projection.sql",
		sourceDeclarationProjectionMigration,
		exactSourceResponseAuthorityMigration,
	} {
		raw, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), string(raw)); err != nil {
			t.Fatalf("apply %s with historical outcome: %v", name, err)
		}
	}
	var projectionKind *string
	if err := pool.QueryRow(t.Context(), `
		SELECT projection_kind FROM station_gap_outcomes WHERE id=1
	`).Scan(&projectionKind); err != nil {
		t.Fatal(err)
	}
	if projectionKind != nil {
		t.Fatalf("historical unprojected outcome changed: projection_kind=%q", *projectionKind)
	}
}

func TestExactSourceResponseAuthorityMigrationRejectsHistoricalInnerSpan(t *testing.T) {
	for _, fixture := range exactSourceResponseProjectionFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			applySourceProjectionAuthorityMigrations(t, pool, false)

			startByte := strings.Index(fixture.wrapped, fixture.declaration)
			insertSourceDeclarationProjectionAuthority(
				t, pool, 20, "fragment_generation", fixture.language, fixture.wrapped,
			)
			if err := insertExactSourceProjectionOutcome(
				t, pool, 20, fixture.kind, fixture.wrapped,
				fixture.declaration, startByte,
			); err != nil {
				t.Fatalf("install migration 161 inner-span fixture: %v", err)
			}

			current, err := os.ReadFile("../../migrations/" + exactSourceResponseAuthorityMigration)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), string(current)); err == nil ||
				!strings.Contains(err.Error(), "requires a fresh reset") {
				t.Fatalf("migration accepted historical inner-span authority: %v", err)
			}
		})
	}
}

func TestExactSourceResponseAuthorityMigrationRejectsNewInnerSpan(t *testing.T) {
	for _, fixture := range exactSourceResponseProjectionFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			applySourceProjectionAuthorityMigrations(t, pool, true)

			insertSourceDeclarationProjectionAuthority(
				t, pool, 30, "fragment_generation", fixture.language, fixture.declaration,
			)
			if err := insertExactSourceProjectionOutcome(
				t, pool, 30, fixture.kind, fixture.declaration, fixture.declaration, 0,
			); err != nil {
				t.Fatalf("insert exact full source response: %v", err)
			}

			startByte := strings.Index(fixture.wrapped, fixture.declaration)
			insertSourceDeclarationProjectionAuthority(
				t, pool, 31, "fragment_generation", fixture.language, fixture.wrapped,
			)
			if err := insertExactSourceProjectionOutcome(
				t, pool, 31, fixture.kind, fixture.wrapped,
				fixture.declaration, startByte,
			); err == nil || !strings.Contains(err.Error(), "exact full provider receipt") {
				t.Fatalf("database accepted a new source inner span: %v", err)
			}
		})
	}
}

func TestExactSourceResponseAuthorityMigrationRejectsChangedPriorAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	applySourceProjectionAuthorityMigrations(t, pool, false)
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION require_station_call_receipt_before_gap_outcome()
		RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql
	`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/" + exactSourceResponseAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), string(raw)); err == nil ||
		!strings.Contains(err.Error(), "requires the exact migration 177 schema") {
		t.Fatalf("migration accepted changed prior receipt authority: %v", err)
	}
}

type exactSourceResponseProjectionFixture struct {
	name, kind, language, declaration, wrapped string
}

func exactSourceResponseProjectionFixtures() []exactSourceResponseProjectionFixture {
	return []exactSourceResponseProjectionFixture{
		{
			name: "source declaration", kind: "source_declaration", language: "go",
			declaration: "func Value() int { return 1 }",
			wrapped:     "```go\nfunc Value() int { return 1 }\n```",
		},
		{
			name: "typescript function", kind: "typescript_function", language: "typescript",
			declaration: "function Value(): number { return 1; }",
			wrapped:     "```typescript\nfunction Value(): number { return 1; }\n```",
		},
	}
}

func insertExactSourceProjectionOutcome(
	t *testing.T,
	pool *pgxpool.Pool,
	id int,
	projectionKind string,
	rawResponse string,
	response string,
	startByte int,
) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO station_gap_outcomes (
			id,opening_id,status,response,response_sha256,error,projection_kind,
			call_receipt_sha256,source_response_sha256,source_start_byte,source_end_byte
		) VALUES (
			$1,$1,'resolved',$2,encode(digest($2::text,'sha256'),'hex'),NULL,
			$3,repeat('c',64),encode(digest($4::text,'sha256'),'hex'),$5,$6
		)
	`, id, response, projectionKind, rawResponse, startByte, startByte+len(response))
	return err
}

func applySourceProjectionAuthorityMigrations(
	t *testing.T,
	pool *pgxpool.Pool,
	includeExactAuthority bool,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), legacyStationOutputProjectionSchema); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"095_station_output_artifact_projection.sql",
		sourceDeclarationProjectionMigration,
	} {
		raw, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	if !includeExactAuthority {
		return
	}
	raw, err := os.ReadFile("../../migrations/" + exactSourceResponseAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), string(raw)); err != nil {
		t.Fatalf("apply %s: %v", exactSourceResponseAuthorityMigration, err)
	}
}
