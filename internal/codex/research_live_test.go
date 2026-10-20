package codex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/sessionintent"
)

// Installed-provider qualification: actual disk, native web and sandbox outcomes.
func TestInstalledResearchDiskWebAndDeniedWrites(t *testing.T) {
	if os.Getenv("OMNI_TEST_RESEARCH_LIVE") != "1" {
		t.Skip("installed Research qualification not selected")
	}
	client, model := installedAgentClient(t)
	root := t.TempDir()
	input := filepath.Join(root, "source.txt")
	marker := "RESEARCH_DISK_OBSERVATION_72c19"
	if err := os.WriteFile(input, []byte(marker), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(text string) string { return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'" }
	command := quote(executable) + " -test.run '^TestResearchReadOnlyOperations$' -test.v -- --research-sandbox-fixture " + quote(root)
	question := "Run exactly this deterministic disposable sandbox qualification command using the shell:\n" + command + "\nThe command reads source.txt and attempts replacing it and creating forbidden.txt, asserting actual OS permission errors and reporting observed proof markers. Do not request permissions or use another path. Then perform a live web search for IANA reserved example domains and cite the IANA source. Report observed outcomes and the prior conversation token supplied in background."
	background := sessionintent.ResearchContext{Recent: []sessionintent.ResearchTurn{{Question: "Retain this token", Answer: "RETAINED_RESEARCH_HISTORY_91ad"}}}
	answer, evidence, err := client.Research(t.Context(), root, question, background, codingagent.Route{Provider: "codex", Model: model, Effort: "low", ServiceTier: "default"})
	if err != nil {
		t.Fatal(err)
	}
	var result codingagent.Result
	if err := json.Unmarshal(evidence, &result); err != nil {
		t.Fatal(err)
	}
	disk, web, denied := false, false, false
	kinds := map[string]int{}
	for _, raw := range result.Evidence {
		var frame struct {
			Method string `json:"method"`
			Params struct {
				Item struct {
					Type             string `json:"type"`
					Status           string `json:"status"`
					Command          string `json:"command"`
					ExitCode         *int   `json:"exitCode"`
					AggregatedOutput string `json:"aggregatedOutput"`
				} `json:"item"`
				Delta string `json:"delta"`
			} `json:"params"`
		}
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		kinds[frame.Method+":"+frame.Params.Item.Type]++
		output := ""
		if frame.Method == "item/completed" && frame.Params.Item.Type == "commandExecution" && frame.Params.Item.ExitCode != nil && *frame.Params.Item.ExitCode == 0 && strings.Contains(frame.Params.Item.Command, executable) && strings.Contains(frame.Params.Item.Command, "TestResearchReadOnlyOperations") && strings.Contains(frame.Params.Item.Command, root) {
			output = frame.Params.Item.AggregatedOutput
		}
		if strings.Contains(output, marker) {
			disk = true
		}
		if strings.Contains(output, "RESEARCH_REPLACE_DENIED") && strings.Contains(output, "RESEARCH_CREATE_DENIED") {
			denied = true
		}
		if frame.Method == "item/completed" && frame.Params.Item.Type == "webSearch" {
			web = true
		}
	}
	bytes, err := os.ReadFile(input)
	if err != nil || string(bytes) != marker {
		t.Fatal("sandbox allowed source mutation", err)
	}
	if _, err := os.Stat(filepath.Join(root, "forbidden.txt")); !os.IsNotExist(err) {
		t.Fatal("sandbox allowed creation", err)
	}
	if !disk || !web || !denied || !strings.Contains(answer, "RETAINED_RESEARCH_HISTORY_91ad") || !strings.Contains(strings.ToLower(answer), "iana.org") {
		t.Logf("native evidence kinds: %v; fixture answer: %s", kinds, answer[:min(len(answer), 2000)])
		t.Fatalf("runtime proof incomplete: disk=%v web=%v denied=%v retained_history=%v cited_iana=%v", disk, web, denied, strings.Contains(answer, "RETAINED_RESEARCH_HISTORY_91ad"), strings.Contains(strings.ToLower(answer), "iana.org"))
	}
	t.Log("Actual native disk observation, completed web search with IANA citation, retained history, write denial and unchanged/absent fixture files verified")
}

// Invoked only by the real Research native shell inside its read-only sandbox.
// Success markers require actual syscalls to fail; model prose cannot satisfy it.
func TestResearchReadOnlyOperations(t *testing.T) {
	root := ""
	for index, arg := range os.Args {
		if arg == "--research-sandbox-fixture" && index+1 < len(os.Args) {
			root = os.Args[index+1]
		}
	}
	if root == "" {
		t.Skip("native sandbox operation fixture not selected")
	}
	input := filepath.Join(root, "source.txt")
	before, err := os.ReadFile(input)
	if err != nil || string(before) != "RESEARCH_DISK_OBSERVATION_72c19" {
		t.Fatal("disk observation failed", err)
	}
	t.Log(string(before))
	for _, attempt := range []struct{ file, proof string }{{input, "RESEARCH_REPLACE_DENIED"}, {filepath.Join(root, "forbidden.txt"), "RESEARCH_CREATE_DENIED"}} {
		err := os.WriteFile(attempt.file, []byte("changed"), 0600)
		if !errors.Is(err, syscall.EROFS) && !errors.Is(err, os.ErrPermission) {
			t.Fatalf("write was not denied by OS permissions: %s: %v", attempt.proof, err)
		}
		t.Log(attempt.proof)
	}
	after, err := os.ReadFile(input)
	if err != nil || string(after) != string(before) {
		t.Fatal("source changed", err)
	}
	if _, err := os.Stat(filepath.Join(root, "forbidden.txt")); !os.IsNotExist(err) {
		t.Fatal("forbidden file exists", err)
	}
}
