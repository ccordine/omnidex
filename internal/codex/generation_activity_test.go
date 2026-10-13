package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type observedSemanticClient interface {
	GeneratePreparedExactWithActivity(context.Context, llm.PreparedModel, func(model.AgentActivity) error) (llm.PreparedGeneration, error)
}

func semanticActivityProtocolClient(t *testing.T, summary string) *Client {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("private semantic protocol fixture requires Python")
	}
	root := t.TempDir()
	command := filepath.Join(root, "semantic-protocol")
	script := "#!" + python + "\nsummary=" + strconv.Quote(summary) + "\n" + `import json,sys
def notify(method,params):
    print(json.dumps({"method":method,"params":dict({"threadId":"thread","turnId":"turn"},**params)}),flush=True)
for line in sys.stdin:
    request=json.loads(line)
    method=request["method"]
    if method=="initialized": continue
    result={}
    if method=="config/read": result={"config":{"mcp_servers":{"private-fixture":{"command":"not-invoked"}}}}
    if method=="thread/start":
        params=request["params"]
        assert params["model"]=="fixture-model" and params["allowProviderModelFallback"] is False
        assert params["sandbox"]=="read-only" and params["approvalPolicy"]=="never"
        assert params["dynamicTools"]==[] and params["environments"]==[] and params["runtimeWorkspaceRoots"]==[]
        assert params["config"]["project_doc_max_bytes"]==0 and params["config"]["mcp_servers"]=={"private-fixture":{"enabled":False}}
        assert all(params["config"]["features"][key] is False for key in ["shell_tool","unified_exec","apps","plugins","multi_agent","hooks"])
        result={"model":"fixture-model","thread":{"id":"thread"}}
    if method=="turn/start":
        params=request["params"]
        assert params["summary"]==summary and params["effort"]=="low" and params["model"]=="fixture-model"
        assert params["input"]==[{"type":"text","text":"Return only RESULT."}]
        assert params["sandboxPolicy"]=={"type":"readOnly","networkAccess":False} and params["approvalPolicy"]=="never"
        assert params["environments"]==[] and params["runtimeWorkspaceRoots"]==[]
        result={"turn":{"id":"turn"}}
    print(json.dumps({"id":request["id"],"result":result}),flush=True)
    if method=="turn/start":
        notify("item/started",{"item":{"id":"summary","type":"reasoning"}})
        notify("item/reasoning/textDelta",{"itemId":"summary","delta":"PRIVATE_REASONING"})
        notify("item/reasoning/summaryTextDelta",{"itemId":"summary","summaryIndex":0,"delta":"Comparing the supplied "})
        notify("item/completed",{"item":{"id":"summary","type":"reasoning","summary":["Comparing the supplied constraints."],"content":["PRIVATE_REASONING"]}})
        notify("item/started",{"item":{"id":"commentary","type":"agentMessage","phase":"commentary"}})
        notify("item/agentMessage/delta",{"itemId":"commentary","delta":"Checking the stated "})
        notify("item/completed",{"item":{"id":"commentary","type":"agentMessage","phase":"commentary","text":"Checking the stated boundary."}})
        notify("item/started",{"item":{"id":"result","type":"agentMessage","phase":"final_answer"}})
        notify("item/agentMessage/delta",{"itemId":"result","delta":"RESULT"})
        notify("item/completed",{"item":{"id":"result","type":"agentMessage","phase":"final_answer","text":"RESULT"}})
        notify("thread/tokenUsage/updated",{"tokenUsage":{"last":{"inputTokens":64,"outputTokens":5,"reasoningOutputTokens":1,"totalTokens":69}}})
        notify("turn/completed",{"turn":{"id":"turn","status":"completed"}})
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client, err := New(command, root, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSemanticGenerationStreamsOnlyPublicActivityBeforeItsExactResult(t *testing.T) {
	client := semanticActivityProtocolClient(t, "detailed")
	observed, ok := any(client).(observedSemanticClient)
	if !ok {
		t.Fatal("semantic Codex cannot stream public activity through a task-local observer")
	}
	prepared := nativeTestRequest("fixture-model", "Return only RESULT.")
	before, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	returned := false
	var activities []model.AgentActivity
	result, err := observed.GeneratePreparedExactWithActivity(context.Background(), prepared, func(activity model.AgentActivity) error {
		if returned {
			t.Fatal("semantic public activity arrived after the final result")
		}
		if err := activity.Validate(); err != nil {
			return err
		}
		activities = append(activities, activity)
		return nil
	})
	returned = true
	if err != nil {
		t.Fatal(err)
	}
	assertSemanticActivityExactResult(t, prepared, result)
	after, _ := llm.ExactPreparedRequestBytes(prepared)
	if string(before) != string(after) {
		t.Fatal("public observation changed the exact prepared semantic request")
	}
	if len(activities) != 4 {
		t.Fatalf("semantic public summary/commentary stream lost its deltas or completion: %+v", activities)
	}
	var public string
	for _, activity := range activities {
		if activity.Role != "plan" || activity.TaskID != "" || activity.Source != model.AgentActivityNative || activity.ThreadID != "thread" || activity.TurnID != "turn" {
			t.Fatalf("semantic public activity invented execution authority: %+v", activity)
		}
		public += activity.Text
	}
	if public != "Comparing the supplied constraints.Checking the stated boundary." || strings.Contains(public, "PRIVATE") || strings.Contains(public, "RESULT") {
		t.Fatalf("private reasoning or raw station result entered public activity: %q", public)
	}
}

func TestSemanticGenerationWithoutObserverPreservesOriginalSummaryAndResponse(t *testing.T) {
	client := semanticActivityProtocolClient(t, "none")
	prepared := nativeTestRequest("fixture-model", "Return only RESULT.")
	result, err := client.GeneratePreparedExact(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	assertSemanticActivityExactResult(t, prepared, result)
}

func assertSemanticActivityExactResult(t *testing.T, prepared llm.PreparedModel, result llm.PreparedGeneration) {
	t.Helper()
	if err := llm.ValidateExactPreparedGenerationForRequest(prepared, result); err != nil {
		t.Fatal(err)
	}
	if result.Content != "RESULT" || result.Usage.PromptEvalCount != 64 || result.Usage.EvalCount != 4 || result.Usage.ReasoningEvalCount != 1 {
		t.Fatalf("public observation changed the exact semantic result or usage: %+v", result)
	}
	var capture llm.CodexResponseCapture
	if err := json.Unmarshal(result.ProviderResponseCapture, &capture); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result.ProviderResponseCapture), "PRIVATE_REASONING") || strings.Contains(string(result.ProviderResponseCapture), "commentary") {
		t.Fatal("public observation changed the original selected completion evidence")
	}
	continuation, err := llm.DecodeCodexRetainedContext(result.ProviderResponseCapture)
	if err != nil {
		t.Fatal(err)
	}
	context, err := llm.ParseCodexContinuation(continuation)
	if err != nil || context.ThreadID != "thread" || context.TurnID != "turn" || context.Model != "fixture-model" {
		t.Fatalf("public observation changed retained native context: %+v %v", context, err)
	}
}
