package queue

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyPublicCutoverHasNoResetCopyOrFallbackPath(t *testing.T) {
	files := []string{
		"legacy_public_cutover.go",
		"legacy_public_cutover_verify.go",
		"legacy_public_preflight.go",
		"legacy_public_object_authority.go",
		"legacy_public_upgrade.go",
		"legacy_public_receipt.go",
	}
	var source strings.Builder
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
	}
	upper := strings.ToUpper(source.String())
	for _, forbidden := range []string{
		"DROP SCHEMA", "MIGRATEFRESH", "CREATE TABLE AS", "INSERT INTO SELECT",
		"CONNECTRUNTIME", "COPY PUBLIC.", "CREATE DATABASE",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("legacy cutover contains forbidden alternate path %q", forbidden)
		}
	}
	for _, required := range []string{
		"ALTER SCHEMA PUBLIC RENAME TO", "PG_ADVISORY_XACT_LOCK",
		"ACCESS EXCLUSIVE", "SERIALIZABLE", "SET SCHEMA PUBLIC",
	} {
		if !strings.Contains(upper, required) {
			t.Fatalf("legacy cutover omits required authority %q", required)
		}
	}
}
