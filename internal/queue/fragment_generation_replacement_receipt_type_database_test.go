package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReplacementRejectsStringTypedOutputLimitReceiptFields(t *testing.T) {
	fixture := newReplacementTransitionFixture(t, "replacement-string-typed-receipt")
	receiptID := fixture.gap.ReplacementOrigin.CallReceiptID
	var original string
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT generation_json FROM station_call_receipts WHERE id=$1
	`, receiptID).Scan(&original); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		"provider_http_status",
		"provider_response_complete",
		"provider_response_bytes_known",
		"provider_response_bytes",
		"provider_response_captured_bytes",
		"provider_done_present",
		"provider_done",
		"usage_present",
		"provider_content_encoding.values",
		"provider_content_encoding.complete",
		"provider_content_encoding.bytes",
		"provider_content_encoding.captured_bytes",
		"provider_content_encoding.uncompressed",
		"usage.prompt_eval_count",
		"usage.eval_count",
		"usage.total_duration_nanos",
		"usage.load_duration_nanos",
		"usage.prompt_eval_duration_nanos",
		"usage.eval_duration_nanos",
	}
	for _, path := range paths {
		rewriteReceiptFieldAsJSONString(t, fixture.pool, receiptID, path)
		var valueType string
		if err := fixture.pool.QueryRow(t.Context(), `
			SELECT jsonb_typeof(
				generation_json::jsonb #> string_to_array($2,'.')
			) FROM station_call_receipts WHERE id=$1
		`, receiptID, path).Scan(&valueType); err != nil {
			t.Fatal(err)
		}
		openingErr, replacementRows, discoveryRows :=
			attemptReplacementGapDiscovery(t, fixture)
		writeReceiptJSON(t, fixture.pool, receiptID, original)

		if valueType != "string" {
			t.Fatalf("rewritten %s type=%q", path, valueType)
		}
		if openingErr == nil || !strings.Contains(
			openingErr.Error(), "exact persisted failed output-limit origin",
		) {
			t.Fatalf("string-typed %s replacement error=%v", path, openingErr)
		}
		if replacementRows != 0 || discoveryRows != 0 {
			t.Fatalf("string-typed %s left %d/%d gap/discovery rows",
				path, replacementRows, discoveryRows)
		}
	}
}

func TestPostgresReplacementRejectsNoncanonicalOutputLimitReceiptLexemes(t *testing.T) {
	fixture := newReplacementTransitionFixture(t, "replacement-noncanonical-receipt")
	receiptID := fixture.gap.ReplacementOrigin.CallReceiptID
	var original, originalEvidence string
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT receipt.generation_json,evidence.response
		FROM station_call_receipts AS receipt
		JOIN llm_call_evidence AS evidence
		  ON evidence.station_call_opening_id=receipt.opening_id
		WHERE receipt.id=$1
	`, receiptID).Scan(&original, &originalEvidence); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, before, after, evidence string
	}{
		{
			name: "numeric exponent", before: `"eval_count":7`,
			after: `"eval_count":1e0`,
		},
		{
			name:   "whitespace content encoding",
			before: `"provider_content_encoding":{"bytes":0,"captured_base64":""`,
			after:  `"provider_content_encoding":{"bytes":0,"captured_base64":" \t\r\n"`,
		},
		{
			name: "non-string content", before: `"content":"partial source"`,
			after: `"content":7`, evidence: "7",
		},
	}
	for _, testCase := range cases {
		if strings.Count(original, testCase.before) != 1 {
			t.Fatalf("%s fixture occurrence count=%d", testCase.name,
				strings.Count(original, testCase.before))
		}
		mutated := strings.Replace(original, testCase.before, testCase.after, 1)
		writeReceiptJSON(t, fixture.pool, receiptID, mutated)
		if testCase.evidence != "" {
			writeReplacementReceiptEvidenceResponse(
				t, fixture.pool, receiptID, testCase.evidence,
			)
		}

		openingErr, replacementRows, discoveryRows :=
			attemptReplacementGapDiscovery(t, fixture)
		writeReceiptJSON(t, fixture.pool, receiptID, original)
		if testCase.evidence != "" {
			writeReplacementReceiptEvidenceResponse(
				t, fixture.pool, receiptID, originalEvidence,
			)
		}

		if openingErr == nil || !strings.Contains(
			openingErr.Error(), "exact persisted failed output-limit origin",
		) {
			t.Fatalf("%s replacement error=%v", testCase.name, openingErr)
		}
		if replacementRows != 0 || discoveryRows != 0 {
			t.Fatalf("%s left %d/%d gap/discovery rows",
				testCase.name, replacementRows, discoveryRows)
		}
	}
}

func attemptReplacementGapDiscovery(
	t *testing.T,
	fixture replacementTransitionFixture,
) (error, int, int) {
	t.Helper()
	_, openingErr := fixture.repository.OpenStationGapDiscovery(
		t.Context(), StationGapDiscoveryOpenRecord{
			Gap: fixture.gap,
			Selection: llm.ProviderIdentitySelection{
				Model: fixture.originModel, NativeContextLimit: fixture.gap.ContextTokens,
			},
		},
	)
	var gaps, discoveries int
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT COUNT(DISTINCT gap.id),COUNT(DISTINCT discovery.id)
		FROM station_gap_openings AS gap
		LEFT JOIN station_provider_discoveries AS discovery
		  ON discovery.gap_opening_id=gap.id
		WHERE gap.work_kind='fragment_generation_replacement'
	`).Scan(&gaps, &discoveries); err != nil {
		t.Fatal(err)
	}
	return openingErr, gaps, discoveries
}

func rewriteReceiptFieldAsJSONString(
	t *testing.T,
	pool *pgxpool.Pool,
	receiptID int64,
	path string,
) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE station_call_receipts
		DISABLE TRIGGER station_call_receipts_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		WITH rewritten AS (
			SELECT jsonb_set(
				generation_json::jsonb,
				string_to_array($2,'.'),
				to_jsonb(generation_json::jsonb #>> string_to_array($2,'.')),
				false
			)::text AS generation_json
			FROM station_call_receipts WHERE id=$1
		)
		UPDATE station_call_receipts AS receipt
		SET generation_json=rewritten.generation_json,
			generation_sha256=encode(
				digest(rewritten.generation_json,'sha256'),'hex'
			)
		FROM rewritten WHERE receipt.id=$1
	`, receiptID, path); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE station_call_receipts
		ENABLE TRIGGER station_call_receipts_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func writeReceiptJSON(
	t *testing.T,
	pool *pgxpool.Pool,
	receiptID int64,
	generationJSON string,
) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE station_call_receipts
		DISABLE TRIGGER station_call_receipts_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE station_call_receipts
		SET generation_json=$2,
			generation_sha256=encode(digest($2,'sha256'),'hex')
		WHERE id=$1
	`, receiptID, generationJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE station_call_receipts
		ENABLE TRIGGER station_call_receipts_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func writeReplacementReceiptEvidenceResponse(
	t *testing.T,
	pool *pgxpool.Pool,
	receiptID int64,
	response string,
) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE llm_call_evidence
		DISABLE TRIGGER llm_call_evidence_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE llm_call_evidence AS evidence
		SET response=$2,response_sha256=encode(digest($2,'sha256'),'hex')
		FROM station_call_receipts AS receipt
		WHERE receipt.id=$1
		  AND evidence.station_call_opening_id=receipt.opening_id
	`, receiptID, response); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE llm_call_evidence
		ENABLE TRIGGER llm_call_evidence_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}
