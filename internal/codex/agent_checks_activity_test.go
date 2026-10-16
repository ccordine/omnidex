package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// This private protocol fixture performs no workload command or inference.
func TestAgentCheckActivityReportsEachActualDispatchOutputAndExit(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("private protocol fixture requires Python")
	}
	scope := agentTestScope(t)
	command := filepath.Join(scope.Root, "check-protocol")
	script := "#!" + python + "\n" + `import base64,json,os,sys
for line in sys.stdin:
    request=json.loads(line)
    if request.get("method")=="initialize":
        print(json.dumps({"id":request["id"],"result":{}}),flush=True)
    if request.get("method")=="command/exec":
        policy=request["params"]["sandboxPolicy"]
        assert policy=={"type":"dangerFullAccess"}
        assert "env" not in request["params"]
        assert "permissionProfile" not in request["params"]
        process=request["params"].get("processId","missing")
        print(json.dumps({"method":"command/exec/outputDelta","params":{"processId":process,"stream":"stdout","deltaBase64":base64.b64encode("streamed ".encode()).decode()}}),flush=True)
        print(json.dumps({"id":request["id"],"result":{"exitCode":0,"stdout":"streamed complete\n","stderr":""}}),flush=True)
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client, err := New(command, scope.Root, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checks := []codingagent.Check{{ID: "first", Argv: []string{"fixture", "first"}}, {ID: "second", Argv: []string{"fixture", "second"}}}
	var events []model.AgentActivity
	results, err := client.ExecuteChecks(context.Background(), scope, checks, func(activity model.AgentActivity) error {
		if err := activity.Validate(); err != nil {
			return err
		}
		events = append(events, activity)
		return nil
	})
	if err != nil || len(results) != 2 {
		t.Fatalf("private protocol check failed: results=%+v error=%v", results, err)
	}
	if len(events) != 8 {
		t.Fatalf("actual independent check dispatches/output/exits disappeared: events=%+v", events)
	}
	for index, check := range checks {
		part := events[index*4 : index*4+4]
		if part[0].Source != model.AgentActivityVerification || part[0].CheckID != check.ID || part[0].Status != model.AgentActivityStarted || !slices.Equal(part[0].Argv, check.Argv) || part[0].CWD != scope.Root || part[0].ThreadID != "" {
			t.Fatalf("check start invented native identity or changed frozen command: %+v", part[0])
		}
		if part[1].Text+part[2].Text != "streamed complete\n" || part[3].Status != model.AgentActivityCompleted || part[3].ExitCode == nil || *part[3].ExitCode != 0 {
			t.Fatalf("check stream lost output remainder or actual completion: %+v", part)
		}
	}
}

func TestAgentCheckLongAcceptedArgvDispatchesUnchangedWithBoundedActivity(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("private protocol fixture requires Python")
	}
	scope := agentTestScope(t)
	command := filepath.Join(scope.Root, "long-check-protocol")
	script := "#!" + python + "\n" + `import json,sys
for line in sys.stdin:
    request=json.loads(line)
    if request.get("method")=="initialize":
        print(json.dumps({"id":request["id"],"result":{}}),flush=True)
    if request.get("method")=="command/exec":
        expected=["fixture-long","界"*2000]+["argument-"+str(index) for index in range(128)]
        exact=request["params"]["command"]==expected
        print(json.dumps({"id":request["id"],"result":{"exitCode":0 if exact else 9,"stdout":"exact full argv dispatched\n" if exact else "command was clipped","stderr":""}}),flush=True)
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client, err := New(command, scope.Root, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{"fixture-long", strings.Repeat("界", 2000)}
	for index := range 128 {
		argv = append(argv, fmt.Sprintf("argument-%d", index))
	}
	var events []model.AgentActivity
	results, err := client.ExecuteChecks(context.Background(), scope, []codingagent.Check{{ID: "long-check", Argv: argv}}, func(activity model.AgentActivity) error { events = append(events, activity); return nil })
	if err != nil || len(results) != 1 || results[0].ExitCode != 0 || results[0].Output != "exact full argv dispatched\n" {
		t.Fatalf("display bounds blocked or changed an accepted actual command: results=%+v error=%v", results, err)
	}
	if len(events) != 3 || !events[0].Truncated || events[0].Status != model.AgentActivityStarted || len(events[0].Argv) > 128 || len(events[0].Argv[1]) > 4096 || events[2].Status != model.AgentActivityCompleted {
		t.Fatalf("long command activity lost explicit bounded presentation/lifecycle: %+v", events)
	}
}

func TestAgentCheckStreamingCompletionRetainsOutputWithoutAggregate(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("private protocol fixture requires Python")
	}
	scope := agentTestScope(t)
	command := filepath.Join(scope.Root, "stream-only-check-protocol")
	script := "#!" + python + "\n" + `import base64,json,sys
for line in sys.stdin:
    request=json.loads(line)
    if request.get("method")=="initialize":
        print(json.dumps({"id":request["id"],"result":{}}),flush=True)
    if request.get("method")=="command/exec":
        assert request["params"]["streamStdoutStderr"] is True
        process=request["params"]["processId"]
        for stream,text in [("stdout","streamed output\n"),("stderr","streamed diagnostic\n")]:
            print(json.dumps({"method":"command/exec/outputDelta","params":{"processId":process,"stream":stream,"deltaBase64":base64.b64encode(text.encode()).decode()}}),flush=True)
        print(json.dumps({"id":request["id"],"result":{"exitCode":0,"stdout":"","stderr":""}}),flush=True)
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client, err := New(command, scope.Root, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var events []model.AgentActivity
	results, err := client.ExecuteChecks(context.Background(), scope, []codingagent.Check{{ID: "stream-only", Argv: []string{"fixture"}}}, func(activity model.AgentActivity) error { events = append(events, activity); return nil })
	if err != nil || len(results) != 1 || results[0].ExitCode != 0 || results[0].Output != "streamed output\nstreamed diagnostic\n" {
		t.Fatalf("valid stream-only native completion lost actual output or completion: results=%+v error=%v", results, err)
	}
	if len(events) != 4 || events[3].Status != model.AgentActivityCompleted || events[3].ExitCode == nil || *events[3].ExitCode != 0 {
		t.Fatalf("stream-only completion lost its actual terminal event: %+v", events)
	}
	if events[1].Text+events[2].Text != results[0].Output {
		t.Fatal("stream-only terminal result differs from validated public output")
	}
}

func TestAgentCheckNonemptyAggregateMustMatchItsStreamedPrefix(t *testing.T) {
	stream := checkActivityStream{call: codingagent.Assignment{Role: codingagent.Review, Task: &codingagent.Task{ID: "task"}}, check: codingagent.Check{ID: "check"}}
	if err := stream.output(0, []byte("observed output"), false); err != nil {
		t.Fatal(err)
	}
	if err := stream.completeOutput("contradictory aggregate", ""); err == nil {
		t.Fatal("a nonempty aggregate replaced validated native output")
	}
}
