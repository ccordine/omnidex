package codex

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativePlanCanonicalizesOnlyLeadingDotFilePaths(t *testing.T) {
	raw := json.RawMessage(`{"tasks":[{"id":"source","goal":"build","acceptance":["works"],"read_files":["./go.mod"],"write_files":["././cmd/main.go"],"dependencies":[],"requirement_ids":["r1"],"checks":[{"id":"test","argv":["go","test","./..."]}],"unknown":"retained"}],"checks":[{"id":"all","argv":["go","test","./..."]}],"questions":[]}`)
	mapped, err := mapAgentPlanResult(codingagent.Assignment{Role: codingagent.Plan}, raw)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(mapped, &document); err != nil {
		t.Fatal(err)
	}
	var tasks []map[string]json.RawMessage
	if err := json.Unmarshal(document["tasks"], &tasks); err != nil {
		t.Fatal(err)
	}
	var reads, writes []string
	if err := json.Unmarshal(tasks[0]["read_files"], &reads); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tasks[0]["write_files"], &writes); err != nil {
		t.Fatal(err)
	}
	if len(reads) != 1 || reads[0] != "go.mod" || len(writes) != 1 || writes[0] != "cmd/main.go" || tasks[0]["unknown"] == nil {
		t.Fatalf("native plan mapping changed more than leading-dot paths: %s", mapped)
	}
}

func TestNativePlanCanonicalizationDoesNotMakeTraversalValid(t *testing.T) {
	raw := json.RawMessage(`{"tasks":[{"id":"source","goal":"build","acceptance":["works"],"read_files":[],"write_files":["./../escape"],"dependencies":[],"requirement_ids":["r1"],"checks":[{"id":"test","argv":["true"]}]}],"checks":[{"id":"all","argv":["true"]}],"questions":[]}`)
	mapped, err := mapAgentPlanResult(codingagent.Assignment{Role: codingagent.Plan}, raw)
	if err != nil {
		t.Fatal(err)
	}
	var workload codingagent.Workload
	if err := json.Unmarshal(mapped, &workload); err != nil {
		t.Fatal(err)
	}
	if err := workload.Validate(); err == nil {
		t.Fatal("leading-dot normalization made traversal valid")
	}
}
