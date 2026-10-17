package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestTaskInstructionReachesNativeTurnVerbatimWithExistingContext(t *testing.T) {
	for _, fixture := range []struct{ name, instruction string }{
		{"checksum", "  Calculate SHA-256 for input.bin.\nWrite the digest to ~/Exports/checksum.txt exactly.  "},
		{"restore", "Restore the archive into /srv/recovered-files.\nPreserve the original bytes and names, including café.txt."},
	} {
		for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
			t.Run(fixture.name+"/"+string(role), func(t *testing.T) {
				input := &agentStartInput{}
				connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(
					`{"id":1,"result":{}}` + "\n" + `{"id":2,"result":{"turn":{"id":"turn"}}}` + "\n"))}
				call := codingagent.Assignment{Role: role,
					Task:    &codingagent.Task{ID: "code-owned-task", Goal: fixture.instruction},
					Context: "Constraints: preserve unrelated files. Acceptance: exact original bytes. Files: input.bin. Owner answer: preserve existing output."}
				prompt, err := renderAgentAssignment(call)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := connection.injectAgentContext(call, "thread"); err != nil {
					t.Fatal(err)
				}
				thread := threadResult{}
				thread.Thread.ID = "thread"
				if _, _, _, err := connection.startAgentTurn(call, thread, prompt); err != nil {
					t.Fatal(err)
				}
				decoder := json.NewDecoder(bytes.NewReader(input.Bytes()))
				var history struct {
					Params struct {
						Items []agentHistoryMessage `json:"items"`
					} `json:"params"`
				}
				if err := decoder.Decode(&history); err != nil {
					t.Fatal(err)
				}
				if len(history.Params.Items) != 1 || len(history.Params.Items[0].Content) != 1 ||
					!strings.Contains(history.Params.Items[0].Content[0].Text, call.Context) {
					t.Fatal("existing constraints, acceptance, files or owner answers were dropped")
				}
				var turn struct {
					Method string `json:"method"`
					Params struct {
						Input []struct{ Text string } `json:"input"`
					} `json:"params"`
				}
				if err := decoder.Decode(&turn); err != nil {
					t.Fatal(err)
				}
				if turn.Method != "turn/start" || len(turn.Params.Input) != 1 {
					t.Fatalf("missing native work turn: %+v", turn)
				}
				actual := turn.Params.Input[0].Text
				if strings.Count(actual, fixture.instruction) != 1 || strings.Contains(actual, call.Context) || strings.Contains(actual, call.Task.ID) {
					t.Fatalf("native turn lost or rewrote the instruction, or mixed in bookkeeping/history: %q", actual)
				}
				if role == codingagent.Execute && actual != fixture.instruction {
					t.Fatalf("execution request differs from its ledger instruction: %q", actual)
				}
				if role == codingagent.Review && !strings.HasSuffix(actual, fixture.instruction) {
					t.Fatalf("review omitted the exact work being reviewed: %q", actual)
				}
			})
		}
	}
}

func TestNativeTaskWithoutWorkInstructionFailsInsteadOfUsingGenericRequest(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		for _, task := range []*codingagent.Task{nil, {}, {Goal: " \n\t "}} {
			prompt, err := renderAgentAssignment(codingagent.Assignment{Role: role, Task: task, Context: "Background alone is not a work instruction."})
			if err == nil || prompt != "" {
				t.Fatalf("%s accepted missing work instruction: prompt=%q error=%v", role, prompt, err)
			}
		}
	}
}
