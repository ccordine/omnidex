package omni

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOmniLedgerExportCommandWritesSessionLedger(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	sessionRoot := filepath.Join(tmp, "sessions")
	outPath := filepath.Join(tmp, "ledger.json")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(sessionRoot)
	session, _, err := store.LoadOrCreate(workspace)
	if err != nil {
		t.Fatal(err)
	}
	session.Turns = append(session.Turns, Turn{
		ID:        "turn_000001",
		UserInput: "run pwd",
		Response:  "done",
		Events: []Event{{
			Type: "structured_command_finished",
			Details: map[string]string{
				"step":      "1",
				"command":   "pwd",
				"exit_code": "0",
				"stdout":    workspace,
			},
		}},
	})
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	if err := app.Run([]string{"ledger", "export", "--workspace", workspace, "--session-root", sessionRoot, "--out", outPath}); err != nil {
		t.Fatalf("ledger export failed: %v\nstderr=%s", err, errOut.String())
	}
	blob, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var ledger EvidenceLedger
	if err := json.Unmarshal(blob, &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.Summary.CommandCount != 1 || ledger.Turns[0].Commands[0].Command != "pwd" {
		t.Fatalf("unexpected ledger: %#v", ledger)
	}
}
