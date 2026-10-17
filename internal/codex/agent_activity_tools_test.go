package codex

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestExecutionUsesNativeCommandsAndFileChanges(t *testing.T) {
	for _, kind := range []string{"commandExecution", "fileChange", "plan"} {
		raw := `{"method":"item/started","params":{"item":{"id":"native-item","type":"` + kind + `"}}}`
		frame := decodeAgentTestFrame(t, raw)
		if err := validateAgentItems(frame, codingagent.Execute, map[string]string{}); err != nil {
			t.Fatalf("normal execution rejected %s: %v", kind, err)
		}
		if err := validateAgentItems(frame, codingagent.Review, map[string]string{}); err != nil {
			t.Fatalf("independent review lost its normal native tools %s: %v", kind, err)
		}
	}
	result := codingagent.Result{}
	if err := collectAgentResultItem(&result, codingagent.Execute, "agentMessage", "final_answer", "", "Done. I changed the implementation."); err != nil {
		t.Fatal(err)
	}
	var text string
	if err := json.Unmarshal(result.Content, &text); err != nil || text != "Done. I changed the implementation." {
		t.Fatalf("ordinary execution response changed: %s, %v", result.Content, err)
	}
}
