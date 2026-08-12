package cognitiongauntlet

import (
	"os"
	"strings"
	"testing"
)

func TestFullCognitionDatabaseCleanupIsIdempotentAfterPartialSetup(t *testing.T) {
	raw, err := os.ReadFile("full_cognition_database_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Count(source, `"DROP SCHEMA IF EXISTS "+`) != 2 {
		t.Fatal("isolated database cleanup must tolerate either schema being absent")
	}
	if strings.Contains(source, `"DROP SCHEMA "+`) {
		t.Fatal("isolated database cleanup retains non-idempotent DROP SCHEMA")
	}
}
