package queue

import (
	"os"
	"strings"
	"testing"
)

const stationGapOpeningPortableEnvelopeV2Migration = "167_station_gap_opening_portable_envelope_v2.sql"

const stationGapOpeningPortableEnvelopeV2ConstraintSHA256 = "2224a480e0a2f3b57d1c3edae9f7e82ff1ec8dec1f56097a969c39c2c59e6a19"

func TestStationGapOpeningPortableEnvelopeV2MigrationIsExactAndClosed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + stationGapOpeningPortableEnvelopeV2Migration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"station_gap_openings_portable_envelope_authority",
		"station_gap_openings_portable_job_identity",
		"c409ff59831ba5afce4ab802bd8e1c695c3f01db117bf04456841c007cd60c63",
		"d50107974696b6779d265ef6b03910ce85ccfb0ce84097f88b794e704179b1d3",
		"envelope_validated IS DISTINCT FROM TRUE",
		"identity_validated IS DISTINCT FROM TRUE",
		"portable envelope V2 rejects malformed existing envelope authority",
		") IS DISTINCT FROM TRUE",
		"DROP CONSTRAINT station_gap_openings_portable_envelope_authority",
		"ADD CONSTRAINT station_gap_openings_portable_envelope_v2 CHECK",
		"'schema'-'id'-'kind'-'payload'-'source_projection'='{}'::jsonb",
		"jsonb_typeof(portable_envelope::jsonb->'schema')='string'",
		"jsonb_typeof(portable_envelope::jsonb->'id')='string'",
		"jsonb_typeof(portable_envelope::jsonb->'kind')='string'",
		"IS NOT DISTINCT FROM portable_schema",
		"IS NOT DISTINCT FROM work_id",
		"IS NOT DISTINCT FROM work_kind",
		"IS NOT DISTINCT FROM portable_payload::jsonb",
		"portable_envelope::jsonb->>'source_projection' IN ( 'go','javascript','java','rust','php' )",
		stationGapOpeningPortableEnvelopeV2ConstraintSHA256,
		"portable envelope V2 authority postcondition failed",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("station-gap envelope V2 migration omitted %q", required)
		}
	}
	if count := strings.Count(source, "DROP CONSTRAINT "); count != 1 {
		t.Fatalf("station-gap envelope V2 migration drop count=%d want 1", count)
	}
	if count := strings.Count(source, "ADD CONSTRAINT "); count != 1 {
		t.Fatalf("station-gap envelope V2 migration add count=%d want 1", count)
	}
	for _, forbidden := range []string{
		"DROP CONSTRAINT IF EXISTS", "NOT VALID", "UPDATE ", "DELETE ", "TRUNCATE ",
		"DROP CONSTRAINT station_gap_openings_portable_job_identity",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("station-gap envelope V2 migration contains forbidden authority %q", forbidden)
		}
	}
}
