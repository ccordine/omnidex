package scrum

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestJobMetadataRejectsRemovedCodingScopePolicy(t *testing.T) {
	metadata := JobMetadata{
		CardID:      "metadata-card",
		CardTitle:   "Exercise job metadata",
		ModelConfig: modelconfig.Config{},
		CodingExecution: codingagent.Configuration{
			Mode:     codingagent.Agents,
			Planner:  codingagent.Route{Provider: "codex", Model: "planner"},
			Executor: codingagent.Route{Provider: "codex", Model: "executor"},
			Reviewer: codingagent.Route{Provider: "codex", Model: "reviewer"},
		},
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeJobMetadata(raw)
	if err != nil {
		t.Fatalf("decode Scrum metadata: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["coding_scope_mode"] = json.RawMessage(`"expansive"`)
	withRemovedPolicy, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeJobMetadata(withRemovedPolicy); err == nil || !strings.Contains(err.Error(), "coding_scope_mode") {
		t.Fatalf("removed Scrum scope policy: %v", err)
	}
}
