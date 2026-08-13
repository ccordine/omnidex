package queue

import (
	"strings"
	"testing"
)

func TestMigrationTransactionalBodyRemovesOnlyExactOuterWrapper(t *testing.T) {
	entry := migrationBundleEntry{name: "001_wrapped.sql", body: []byte(
		"BEGIN;\nCREATE TABLE probe (id bigint);\nCOMMIT;\n",
	)}
	body, err := entry.transactionalBody()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "CREATE TABLE probe (id bigint);\n" {
		t.Fatalf("transactional body=%q", body)
	}
	for _, rejected := range []string{
		"SELECT 1;\nCOMMIT;\n",
		"BEGIN;\nSELECT 1;\nCOMMIT;\nCOMMIT;\n",
		"ROLLBACK;\n",
	} {
		entry.body = []byte(rejected)
		if _, err := entry.transactionalBody(); err == nil ||
			!strings.Contains(err.Error(), "nested transaction control") {
			t.Fatalf("transactionalBody(%q) error=%v", rejected, err)
		}
	}
}
