package queue

import (
	"os"
	"strings"
	"testing"
)

func TestStationJSONAuthorityMigrationIsHashGuardedAndNullSafe(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/078_station_json_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"572bbdf8c7469738a13b6add6f857919f81ebdc3647bbba20a6b20ad2b0cc07e",
		"b8dd0f77a20b826373b0862513bc367c92607acdf5cdb955dc601bd21effd9e5",
		"jsonb_typeof(operations) IS DISTINCT FROM 'array'",
		"operation.item->>'operation' IS DISTINCT FROM capture.operation",
		"request_disposition IS NULL",
		"response_disposition IS NULL",
		"NOT envelope ? 'failure_reason'",
		"station call receipt contains sparse JSON authority",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("station JSON authority migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"operation.item->>'operation'<>",
		"receipt_generation->>'protocol'<>",
		"DROP TRIGGER IF EXISTS",
		"CASCADE",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("station JSON authority migration contains unsafe %q", forbidden)
		}
	}
}
