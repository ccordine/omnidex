package queue

import (
	"os"
	"strings"
	"testing"
)

const (
	qwen35ChatMLRawTransportV2Migration = "164_qwen35_chatml_raw_transport_v2.sql"
	qwen35ChatMLTokenizerProfileV2      = "ollama-0.24.0-qwen35-gpt2-chatml-boundary-v2"
	rawTextGenerateProtocolV2           = "omnidex.ollama-raw-text-generate-request.v2"
)

func TestQwen35ChatMLRawTransportV2MigrationIsFreshAndClosed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + qwen35ChatMLRawTransportV2Migration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE",
		"IF EXISTS (SELECT 1 FROM station_gap_openings) OR EXISTS (SELECT 1 FROM station_call_openings)",
		"requires fresh station gap and call state",
		"DROP CONSTRAINT station_call_openings_protocol_check",
		"DROP CONSTRAINT station_call_openings_current_raw_transport",
		"DROP CONSTRAINT station_call_openings_tokenizer_profile_check",
		"ADD CONSTRAINT station_call_openings_protocol_check",
		"ADD CONSTRAINT station_call_openings_current_raw_transport",
		"ADD CONSTRAINT station_call_openings_tokenizer_profile_check",
		"protocol='" + rawTextGenerateProtocolV2 + "'",
		qwen35ChatMLTokenizerProfileV2,
		"pg_get_constraintdef(oid)",
		"convalidated",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Qwen 3.5 ChatML V2 migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"NOT VALID", "UPDATE ", "DELETE ", "TRUNCATE ",
		"LIKE 'ollama-%'", "SIMILAR TO",
		"ollama-0.24.0-roleplay-raw-completion-v1",
		"ollama-0.24.0-roleplay-semantic-completion-v1",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("Qwen 3.5 ChatML V2 migration contains forbidden authority %q", forbidden)
		}
	}
}

func TestPostgresQwen35ChatMLRawTransportV2AuthorityIsExact(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "164"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, qwen35ChatMLRawTransportV2Migration, 1)

	rows, err := pool.Query(t.Context(), `
		SELECT conname,convalidated,pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE connamespace=(SELECT oid FROM pg_namespace WHERE nspname=current_schema())
		  AND conname IN (
			'station_call_openings_protocol_check',
			'station_call_openings_current_raw_transport',
			'station_call_openings_tokenizer_profile_check'
		  )
		ORDER BY conname
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	definitions := make(map[string]string, 3)
	for rows.Next() {
		var name, definition string
		var validated bool
		if err := rows.Scan(&name, &validated, &definition); err != nil {
			t.Fatal(err)
		}
		if !validated {
			t.Fatalf("Qwen 3.5 ChatML V2 constraint %q is not validated", name)
		}
		definitions[name] = definition
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 3 {
		t.Fatalf("Qwen 3.5 ChatML V2 constraints=%d want 3", len(definitions))
	}
	for _, name := range []string{
		"station_call_openings_protocol_check",
		"station_call_openings_current_raw_transport",
	} {
		definition := definitions[name]
		if !strings.Contains(definition, rawTextGenerateProtocolV2) ||
			strings.Contains(definition, "raw-text-generate-request.v1") {
			t.Fatalf("protocol constraint %q is not V2-only: %s", name, definition)
		}
	}
	profileDefinition := definitions["station_call_openings_tokenizer_profile_check"]
	if !strings.Contains(profileDefinition, qwen35ChatMLTokenizerProfileV2) ||
		strings.Contains(profileDefinition, "qwen35-gpt2-boundary-v1") ||
		strings.Contains(profileDefinition, "roleplay-raw-completion-v1") ||
		strings.Contains(profileDefinition, "roleplay-semantic-completion-v1") {
		t.Fatalf("tokenizer-profile constraint is not ChatML V2-only: %s", profileDefinition)
	}
}
