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
}

func TestMigrationTransactionalBodyRejectsEveryPostgresTransactionControlFamily(t *testing.T) {
	entry := migrationBundleEntry{name: "001_nested.sql"}
	for name, rejected := range map[string]string{
		"begin":                           "BEGIN;\n",
		"begin work":                      "BEGIN WORK;\n",
		"begin transaction":               "BEGIN TRANSACTION;\n",
		"start transaction":               "START TRANSACTION;\n",
		"start transaction with mode":     "START /* exact */ TRANSACTION ISOLATION LEVEL SERIALIZABLE;\n",
		"commit":                          "COMMIT;\n",
		"commit work":                     "COMMIT WORK;\n",
		"commit transaction":              "COMMIT TRANSACTION;\n",
		"commit and chain":                "COMMIT AND CHAIN;\n",
		"commit and no chain":             "COMMIT AND NO CHAIN;\n",
		"commit work and chain":           "COMMIT WORK AND CHAIN;\n",
		"end":                             "END;\n",
		"end work":                        "END WORK;\n",
		"end transaction and chain":       "END TRANSACTION AND CHAIN;\n",
		"rollback":                        "ROLLBACK;\n",
		"rollback work":                   "ROLLBACK WORK;\n",
		"rollback transaction":            "ROLLBACK TRANSACTION;\n",
		"rollback and chain":              "ROLLBACK AND CHAIN;\n",
		"rollback and no chain":           "ROLLBACK AND NO CHAIN;\n",
		"abort":                           "ABORT;\n",
		"abort work":                      "ABORT WORK;\n",
		"abort transaction":               "ABORT TRANSACTION;\n",
		"savepoint":                       "SAVEPOINT migration_owned;\n",
		"rollback to":                     "ROLLBACK TO migration_owned;\n",
		"rollback work to savepoint":      "ROLLBACK WORK TO SAVEPOINT migration_owned;\n",
		"release savepoint":               "RELEASE SAVEPOINT migration_owned;\n",
		"release":                         "RELEASE migration_owned;\n",
		"prepare transaction":             "PREPARE TRANSACTION 'migration-owned';\n",
		"commit prepared":                 "COMMIT PREPARED 'migration-owned';\n",
		"rollback prepared":               "ROLLBACK PREPARED 'migration-owned';\n",
		"set transaction":                 "SET TRANSACTION ISOLATION LEVEL READ COMMITTED;\n",
		"set transaction snapshot":        "SET TRANSACTION SNAPSHOT 'migration-owned';\n",
		"set local transaction":           "SET LOCAL TRANSACTION ISOLATION LEVEL READ COMMITTED;\n",
		"set session transaction":         "SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL READ COMMITTED;\n",
		"control after statement":         "SELECT 1; /* exact */ COMMIT WORK;\n",
		"control after line comment":      "SELECT 1; -- exact\nROLLBACK TRANSACTION;\n",
		"control after carriage comment":  "SELECT 1; -- exact\rCOMMIT WORK;\n",
		"comment before control":          "/* exact */ COMMIT AND CHAIN;\n",
		"mixed case control":              "cOmMiT wOrK;\n",
		"vertical space before control":   "\vROLLBACK WORK;\n",
		"control after dollar identifier": "SELECT migration$tag$identifier; COMMIT WORK;\n",
		"control between non-ASCII dollar identifiers": "SELECT 1 AS α$tag$; COMMIT; SELECT 3 AS β$tag$;\n",
		"wrapped inner control":                        "BEGIN;\nSELECT 1;\nCOMMIT AND CHAIN;\nCOMMIT;\n",
		"prepared after line comment":                  "PREPARE -- exact\n TRANSACTION 'migration-owned';\n",
	} {
		t.Run(name, func(t *testing.T) {
			entry.body = []byte(rejected)
			if _, err := entry.transactionalBody(); err == nil ||
				!strings.Contains(err.Error(), "nested transaction control") {
				t.Fatalf("transactionalBody(%q) error=%v", rejected, err)
			}
		})
	}
}

func TestMigrationTransactionalBodyIgnoresNonStatementControlWords(t *testing.T) {
	entry := migrationBundleEntry{name: "001_quoted.sql"}
	for name, accepted := range map[string]string{
		"single quoted":                  "SELECT 'COMMIT WORK; ROLLBACK TRANSACTION;';\n",
		"escape string":                  `SELECT E'COMMIT WORK;\' still quoted';` + "\n",
		"quoted identifier":              `SELECT 1 AS "COMMIT";` + "\n",
		"line comment":                   "-- COMMIT AND CHAIN;\nSELECT 1;\n",
		"nested block comment":           "/* BEGIN; /* ROLLBACK; */ COMMIT; */ SELECT 1;\n",
		"dollar quoted":                  "DO $$ BEGIN RAISE NOTICE 'COMMIT;'; END $$;\n",
		"tagged dollar quoted":           "DO $body$ BEGIN RAISE NOTICE 'ROLLBACK;'; END $body$;\n",
		"dollar in identifier":           "SELECT migration$tag$identifier;\n",
		"dollar in non-ASCII identifier": "SELECT 1 AS α$tag$;\n",
		"prepared statement":             "PREPARE migration_owned AS SELECT 1;\n",
	} {
		t.Run(name, func(t *testing.T) {
			entry.body = []byte(accepted)
			if _, err := entry.transactionalBody(); err != nil {
				t.Fatalf("transactionalBody(%q) error=%v", accepted, err)
			}
		})
	}
}

func TestMigrationTransactionalBodyRejectsUnclosedSQLLexicalRegions(t *testing.T) {
	entry := migrationBundleEntry{name: "001_unclosed.sql"}
	for name, rejected := range map[string]string{
		"single quoted":        "SELECT 'COMMIT WORK;\n",
		"double quoted":        `SELECT "COMMIT WORK;` + "\n",
		"block comment":        "SELECT 1; /* COMMIT WORK;\n",
		"dollar quoted":        "DO $body$ BEGIN COMMIT; END;\n",
		"non-ASCII dollar tag": "SELECT $α$COMMIT;$α$;\n",
	} {
		t.Run(name, func(t *testing.T) {
			entry.body = []byte(rejected)
			if _, err := entry.transactionalBody(); err == nil ||
				!strings.Contains(err.Error(), "SQL lexical boundary") {
				t.Fatalf("transactionalBody(%q) error=%v", rejected, err)
			}
		})
	}
}

func TestMigrationTransactionalBodyRejectsCommitHiddenByNonASCIIDollarTags(t *testing.T) {
	entry := migrationBundleEntry{name: "001_non_ascii_dollar_tag.sql", body: []byte(
		"SELECT $α$'$α$; COMMIT; SELECT $α$'$α$;\n",
	)}
	_, err := entry.transactionalBody()
	if err == nil || !strings.Contains(err.Error(), "non-ASCII dollar-quote tag") {
		t.Fatalf("paired non-ASCII dollar-tag transaction control error=%v", err)
	}
}
