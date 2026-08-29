package queue

import (
	"os"
	"strings"
	"testing"
)

func TestStationOutputArtifactProjectionMigrationBindsExactSourceSpan(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/095_station_output_artifact_projection.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"ADD COLUMN projection_kind TEXT",
		"ADD COLUMN call_receipt_sha256 TEXT",
		"ADD COLUMN source_response_sha256 TEXT",
		"ADD COLUMN source_start_byte INTEGER",
		"ADD COLUMN source_end_byte INTEGER",
		") NOT VALID",
		"migration mutated append-only historical outcomes",
		"station_call_receipts_generation_resource_ceiling",
		"octet_length(generation_json)<=134217728",
		"station_gap_outcomes_projected_response",
		"octet_length(response)<=16777216",
		"substring(convert_to(call_response,'UTF8') FROM NEW.source_start_byte+1",
		"IS DISTINCT FROM convert_to(NEW.response,'UTF8')",
		"NEW.call_receipt_sha256 IS DISTINCT FROM call_receipt_sha256",
		"NEW.source_response_sha256 IS DISTINCT FROM call_response_sha256",
		"gap_work_kind IN ('fragment_generation','fragment_correction')",
		"gap_payload->>'language'='typescript'",
		"NOT (gap_payload ? 'repair_region')",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("station output projection migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_outcomes",
		"NEW.response IS DISTINCT FROM call_response",
		"octet_length(response)<=65536 AND response_sha256",
		"octet_length(generation_json)<=131072)",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("station output projection migration retains forbidden hot-path guard %q", forbidden)
		}
	}
}

func TestStationOutputArtifactProjectionMigrationPreservesAppendOnlyHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, legacyStationOutputProjectionSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, legacyStationOutputProjectionFixture); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile("../../migrations/095_station_output_artifact_projection.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("apply migration 095 to populated append-only history: %v", err)
	}

	var response string
	var projectionKind, receiptSHA, sourceSHA *string
	var startByte, endByte *int
	if err := pool.QueryRow(ctx, `
		SELECT response,projection_kind,call_receipt_sha256,source_response_sha256,
		       source_start_byte,source_end_byte
		FROM station_gap_outcomes WHERE id=1
	`).Scan(&response, &projectionKind, &receiptSHA, &sourceSHA, &startByte, &endByte); err != nil {
		t.Fatal(err)
	}
	if response != "legacy response" || projectionKind != nil || receiptSHA != nil ||
		sourceSHA != nil || startByte != nil || endByte != nil {
		t.Fatalf("migration mutated append-only legacy outcome: response=%q projection=%v/%v/%v/%v/%v",
			response, projectionKind, receiptSHA, sourceSHA, startByte, endByte)
	}

	var validated bool
	if err := pool.QueryRow(ctx, `
		SELECT convalidated FROM pg_constraint
		WHERE conrelid='station_gap_outcomes'::regclass
		  AND conname='station_gap_outcomes_projected_response'
	`).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if validated {
		t.Fatal("projection constraint incorrectly claims legacy rows were rewritten and validated")
	}

	if _, err := pool.Exec(ctx, `UPDATE station_gap_outcomes SET response='changed' WHERE id=1`); err == nil ||
		!strings.Contains(err.Error(), "station gap history is append-only") {
		t.Fatalf("legacy outcome lost append-only protection: %v", err)
	}
	if _, err := pool.Exec(ctx, legacyStationOutputProjectionNewAuthority); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, legacyStationOutputProjectionNewUnprojected); err == nil ||
		!strings.Contains(err.Error(), "projection differs") {
		t.Fatalf("new unprojected outcome bypassed projection authority: %v", err)
	}
	if _, err := pool.Exec(ctx, legacyStationOutputProjectionNewProjected); err != nil {
		t.Fatalf("new exact projected outcome was rejected: %v", err)
	}
}

const legacyStationOutputProjectionSchema = `
CREATE TABLE station_gap_openings (
    id BIGINT PRIMARY KEY, work_kind TEXT NOT NULL, portable_payload TEXT NOT NULL
);
CREATE TABLE station_gap_outcomes (
    id BIGINT PRIMARY KEY, opening_id BIGINT NOT NULL, status TEXT NOT NULL,
    response TEXT, response_sha256 TEXT, error TEXT,
    CONSTRAINT legacy_station_gap_outcomes_response CHECK (
        (status='resolved' AND response IS NOT NULL AND octet_length(response) <= 65536 AND
         response_sha256=encode(digest(response,'sha256'),'hex') AND error IS NULL) OR
        (status='failed' AND response IS NULL AND response_sha256 IS NULL AND error IS NOT NULL)
    )
);
CREATE FUNCTION prevent_station_gap_history_mutation() RETURNS TRIGGER AS $$
BEGIN RAISE EXCEPTION 'station gap history is append-only'; END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_gap_outcomes_immutable BEFORE UPDATE OR DELETE ON station_gap_outcomes
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TABLE station_provider_discoveries (id BIGINT PRIMARY KEY, gap_opening_id BIGINT NOT NULL);
CREATE TABLE station_provider_discovery_receipts (opening_id BIGINT NOT NULL, status TEXT NOT NULL);
CREATE TABLE station_call_openings (id BIGINT PRIMARY KEY, gap_opening_id BIGINT NOT NULL);
CREATE TABLE station_call_receipts (
    opening_id BIGINT NOT NULL, status TEXT NOT NULL, generation_json TEXT NOT NULL,
    generation_sha256 TEXT NOT NULL,
    CONSTRAINT legacy_station_call_receipts_generation CHECK (
        octet_length(generation_json) <= 131072
    )
);
CREATE TABLE station_call_response_captures (id BIGINT PRIMARY KEY);
CREATE TABLE llm_call_evidence (
    id BIGINT PRIMARY KEY, station_call_opening_id BIGINT NOT NULL, response_sha256 TEXT NOT NULL
);
CREATE FUNCTION require_station_call_receipt_before_gap_outcome() RETURNS TRIGGER AS $$
BEGIN RETURN NEW; END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_gap_outcomes_require_call_receipt BEFORE INSERT ON station_gap_outcomes
FOR EACH ROW EXECUTE FUNCTION require_station_call_receipt_before_gap_outcome();`

const legacyStationOutputProjectionFixture = `
INSERT INTO station_gap_openings VALUES (1,'conversation_response','{}');
INSERT INTO station_provider_discoveries VALUES (1,1);
INSERT INTO station_provider_discovery_receipts VALUES (1,'succeeded');
INSERT INTO station_call_openings VALUES (1,1);
INSERT INTO station_call_receipts VALUES (
    1,'succeeded','{"content":"legacy response"}',repeat('a',64)
);
INSERT INTO llm_call_evidence VALUES (
    1,1,encode(digest('legacy response','sha256'),'hex')
);
INSERT INTO station_gap_outcomes VALUES (
    1,1,'resolved','legacy response',encode(digest('legacy response','sha256'),'hex'),NULL
);`

const legacyStationOutputProjectionNewAuthority = `
INSERT INTO station_gap_openings VALUES (2,'conversation_response','{}');
INSERT INTO station_provider_discoveries VALUES (2,2);
INSERT INTO station_provider_discovery_receipts VALUES (2,'succeeded');
INSERT INTO station_call_openings VALUES (2,2);
INSERT INTO station_call_receipts VALUES (
    2,'succeeded','{"content":"new response"}',repeat('b',64)
);
INSERT INTO llm_call_evidence VALUES (
    2,2,encode(digest('new response','sha256'),'hex')
);`

const legacyStationOutputProjectionNewUnprojected = `
INSERT INTO station_gap_outcomes (
    id,opening_id,status,response,response_sha256,error
) VALUES (
    2,2,'resolved','new response',encode(digest('new response','sha256'),'hex'),NULL
);`

const legacyStationOutputProjectionNewProjected = `
INSERT INTO station_gap_outcomes (
    id,opening_id,status,response,response_sha256,error,projection_kind,
    call_receipt_sha256,source_response_sha256,source_start_byte,source_end_byte
) VALUES (
    2,2,'resolved','new response',encode(digest('new response','sha256'),'hex'),NULL,
    'exact_response',repeat('b',64),encode(digest('new response','sha256'),'hex'),0,
    octet_length('new response')
);`
