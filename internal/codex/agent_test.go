package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func agentTestScope(t *testing.T) codingagent.Assignment {
	t.Helper()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"source.go", "contract.txt"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte("fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return codingagent.Assignment{JobID: 1, Generation: 1, Role: codingagent.Execute,
		Root: root, ReadFiles: []string{"contract.txt", "source.go"}, WriteFiles: []string{"source.go"},
		Task:  &codingagent.Task{ID: "source", Goal: "Implement the source", Acceptance: []string{"source works"}, ReadFiles: []string{"contract.txt"}, WriteFiles: []string{"source.go"}},
		Route: codingagent.Route{Provider: "codex", Model: "fixture-model"}}
}

func TestAgentAssignmentRejectsUnboundAuthorityAndExpandedScope(t *testing.T) {
	scope := agentTestScope(t)
	if err := validateAgentAssignment(scope); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*codingagent.Assignment)
	}{
		{"missing job", func(call *codingagent.Assignment) { call.JobID = 0 }},
		{"missing generation", func(call *codingagent.Assignment) { call.Generation = 0 }},
		{"relative root", func(call *codingagent.Assignment) { call.Root = "." }},
		{"foreign provider", func(call *codingagent.Assignment) { call.Route.Provider = "other" }},
		{"unknown role", func(call *codingagent.Assignment) { call.Role = "inspect-and-edit" }},
		{"review write access", func(call *codingagent.Assignment) { call.Role = codingagent.Review }},
		{"planning write access", func(call *codingagent.Assignment) { call.Role = codingagent.PlanningInitial }},
		{"parent traversal", func(call *codingagent.Assignment) { call.ReadFiles = []string{"../secret"} }},
		{"whole root", func(call *codingagent.Assignment) { call.ReadFiles = []string{"."} }},
		{"directory grant", func(call *codingagent.Assignment) { call.ReadFiles = []string{"subdir"} }},
		{"missing file", func(call *codingagent.Assignment) { call.ReadFiles = []string{"absent.go"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := scope
			test.change(&call)
			if err := validateAgentAssignment(call); err == nil {
				t.Fatal("unbound authority or expanded filesystem scope was accepted")
			}
		})
	}
	if err := os.Mkdir(filepath.Join(scope.Root, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	scope.ReadFiles = []string{"subdir"}
	if err := validateAgentAssignment(scope); err == nil {
		t.Fatal("an existing directory became a recursive grant")
	}
	if err := os.Symlink(filepath.Join(scope.Root, "contract.txt"), filepath.Join(scope.Root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	scope.ReadFiles = []string{"alias"}
	if err := validateAgentAssignment(scope); err == nil {
		t.Fatal("a symlink replaced exact physical-file scope")
	}
}

func TestAgentEnvironmentExcludesServiceSecrets(t *testing.T) {
	t.Setenv("OMNI_AGENT_SERVICE_SECRET", "qualification-only-value")
	t.Setenv("DATABASE_URL", "qualification-only-database")
	t.Setenv("REDIS_PASSWORD", "qualification-only-password")
	for _, value := range agentProcessEnvironment() {
		if strings.HasPrefix(value, "OMNI_AGENT_SERVICE_SECRET=") || strings.HasPrefix(value, "DATABASE_URL=") ||
			strings.HasPrefix(value, "REDIS_PASSWORD=") {
			t.Fatal("service authority crossed into the native agent process")
		}
	}
	scratch := t.TempDir()
	environment := agentToolEnvironment(scratch)
	if environment["HOME"] != scratch || environment["TMPDIR"] != scratch || environment["GOMAXPROCS"] != "2" {
		t.Fatal("semantic parser inherited host storage or unbounded Go workers")
	}
	for _, key := range []string{"OMNI_AGENT_SERVICE_SECRET", "DATABASE_URL", "REDIS_PASSWORD", "CODEX_HOME"} {
		if _, found := environment[key]; found {
			t.Fatalf("tool environment exposes %s", key)
		}
	}
}

func TestExecutionInheritsHostSSHAndDockerEnvironment(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/fixture/ssh-agent.sock")
	t.Setenv("DOCKER_HOST", "unix:///fixture/docker.sock")
	t.Setenv("DOCKER_CONTEXT", "fixture-context")
	root := t.TempDir()
	command := filepath.Join(root, "codex-fixture")
	fixture := "#!/bin/sh\nprintf '%s\\n' \"$SSH_AUTH_SOCK\" \"$DOCKER_HOST\" \"$DOCKER_CONTEXT\"\nread -r fixture_input\n"
	if err := os.WriteFile(command, []byte(fixture), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := startExecutionConnection(ctx, command, root)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.close()
	for _, expected := range []string{"/fixture/ssh-agent.sock", "unix:///fixture/docker.sock", "fixture-context"} {
		if !connection.output.Scan() || connection.output.Text() != expected {
			t.Fatalf("execution did not inherit host configuration %q", expected)
		}
	}
	if connection.process.Dir != root || connection.process.Env != nil {
		t.Fatal("execution substituted a filtered environment or working directory")
	}
	if strings.Join(connection.process.Args[1:], " ") != "app-server --listen stdio:// -c project_doc_max_bytes=0" {
		t.Fatalf("native channels must suppress automatic document bloat without restricting tools: %v", connection.process.Args)
	}
}

func decodeAgentTestFrame(t *testing.T, raw string) rpcFrame {
	t.Helper()
	var frame rpcFrame
	if err := json.Unmarshal([]byte(raw), &frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestAgentTransportAllowsOnlyBoundFileTools(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		method  string
		wantErr bool
	}{
		{name: "scoped file tool", method: "item/tool/call"},
		{name: "user question", method: "item/tool/requestUserInput", wantErr: true},
		{name: "command approval", method: "item/commandExecution/requestApproval", wantErr: true},
		{name: "foreign client tool", method: "mcpServer/elicitation/request", wantErr: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(`{"id":1,"method":"` + fixture.method + `","params":{}}` + "\n"))
			connection := &connection{output: scanner}
			_, err := connection.nextAgent()
			if (err != nil) != fixture.wantErr {
				t.Fatalf("method %s error=%v", fixture.method, err)
			}
		})
	}
}

func TestAgentNativeReceiptsRejectForeignIncompleteAndUngrantableEvents(t *testing.T) {
	for _, raw := range []string{
		`{"method":"item/completed","params":{"threadId":"foreign","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"{}"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"foreign","item":{"type":"commandExecution"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"interrupted"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"foreign","status":"completed"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`,
	} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		if finished, err := collectAgentFrame(decodeAgentTestFrame(t, raw), []byte(raw), &result, codingagent.Execute); err == nil || finished || result.Completed {
			t.Fatalf("foreign, incomplete, or ungranted receipt accepted: %s", raw)
		}
	}
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	tool := `{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"type":"mcpToolCall"}}}`
	if finished, err := collectAgentFrame(decodeAgentTestFrame(t, tool), []byte(tool), &result, codingagent.Review); err != nil || finished || result.Completed {
		t.Fatalf("configured MCP activity changed assignment completion: finished=%v error=%v", finished, err)
	}
	result = codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	answer := `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"{}"}}}`
	if finished, err := collectAgentFrame(decodeAgentTestFrame(t, answer), []byte(answer), &result, codingagent.Execute); err != nil || finished || result.Completed {
		t.Fatalf("a final message alone became completion: finished=%v error=%v", finished, err)
	}
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, answer), []byte(answer), &result, codingagent.Execute); err == nil {
		t.Fatal("duplicate structured answers were accepted")
	}
	completed := `{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`
	if finished, err := collectAgentFrame(decodeAgentTestFrame(t, completed), []byte(completed), &result, codingagent.Execute); err != nil || !finished || !result.Completed {
		t.Fatalf("exact native completion rejected: finished=%v error=%v", finished, err)
	}
}

func TestAgentItemLifecycleRejectsWriteAttemptsAndUnboundCompletion(t *testing.T) {
	start := decodeAgentTestFrame(t, `{"method":"item/started","params":{"item":{"id":"item-1","type":"dynamicToolCall"}}}`)
	complete := decodeAgentTestFrame(t, `{"method":"item/completed","params":{"item":{"id":"item-1","type":"dynamicToolCall","status":"completed"}}}`)
	open := map[string]string{}
	if err := validateAgentItems(complete, codingagent.Execute, open); err == nil {
		t.Fatal("completion without a started native item was accepted")
	}
	if err := validateAgentItems(start, codingagent.Execute, open); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentItems(start, codingagent.Execute, open); err == nil {
		t.Fatal("an active native identity was reused")
	}
	unfinished := decodeAgentTestFrame(t, `{"method":"item/completed","params":{"item":{"id":"item-1","type":"dynamicToolCall","status":"inProgress"}}}`)
	if err := validateAgentItems(unfinished, codingagent.Execute, open); err == nil {
		t.Fatal("an unfinished command became completed evidence")
	}
	if err := validateAgentItems(complete, codingagent.Execute, open); err != nil || len(open) != 0 {
		t.Fatalf("completed command not accounted for: %v", err)
	}
	write := decodeAgentTestFrame(t, `{"method":"item/started","params":{"item":{"id":"edit-1","type":"fileChange"}}}`)
	err := validateAgentItems(write, codingagent.PlanningInitial, open)
	if err == nil {
		t.Fatal("planning parser acquired source write authority")
	}
	var retryable interface{ Retryable() bool }
	if !errors.As(err, &retryable) || retryable.Retryable() {
		t.Fatalf("blocked native side effect remained retryable: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("blocked parser edit changed native item authority: %v", open)
	}
}

func TestNativeStructuredFinalAnswerReplacementClosesSupersededItem(t *testing.T) {
	open := map[string]string{}
	frames := []string{
		`{"method":"item/started","params":{"item":{"id":"truncated","type":"agentMessage","phase":"final_answer"}}}`,
		`{"method":"item/agentMessage/delta","params":{"itemId":"truncated","delta":"partial"}}`,
		`{"method":"item/started","params":{"item":{"id":"replacement","type":"agentMessage","phase":"final_answer"}}}`,
		`{"method":"item/agentMessage/delta","params":{"itemId":"replacement","delta":"complete"}}`,
		`{"method":"item/completed","params":{"item":{"id":"replacement","type":"agentMessage","phase":"final_answer","status":"completed"}}}`,
	}
	for _, raw := range frames {
		if err := validateAgentItems(decodeAgentTestFrame(t, raw), codingagent.Plan, open); err != nil {
			t.Fatal(err)
		}
	}
	if len(open) != 0 {
		t.Fatalf("superseded structured answer remained open: %v", open)
	}

	open = map[string]string{
		"first":  nativeItemState("agentMessage", "final_answer"),
		"second": nativeItemState("agentMessage", "final_answer"),
	}
	replacement := `{"method":"item/started","params":{"item":{"id":"third","type":"agentMessage","phase":"final_answer"}}}`
	if err := validateAgentItems(decodeAgentTestFrame(t, replacement), codingagent.Plan, open); err == nil {
		t.Fatal("replacement hid multiple incomplete final answers")
	}
}

func TestAgentReceiptPersistenceMustSucceedBeforeEvidenceIsAccepted(t *testing.T) {
	result := codingagent.Result{}
	raw := json.RawMessage(`{"original":"receipt"}`)
	failure := errors.New("durable evidence unavailable")
	if err := recordAgentFrame(&result, raw, func(codingagent.NativeEvent) error { return failure }); !errors.Is(err, failure) || len(result.Evidence) != 0 {
		t.Fatal("failed persistence was accepted as invocation evidence")
	}
	if err := recordAgentFrame(&result, raw, nil); err != nil {
		t.Fatal(err)
	}
	raw[0] = '!'
	if !json.Valid(result.Evidence[0]) {
		t.Fatal("retained original receipt aliases the transport read buffer")
	}
}

type agentTestToolInput struct{ bytes.Buffer }

func (input *agentTestToolInput) Close() error { return nil }
