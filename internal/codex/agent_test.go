package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
		{"planning write access", func(call *codingagent.Assignment) { call.Role = codingagent.Plan }},
		{"overall review write access", func(call *codingagent.Assignment) { call.Role = codingagent.OverallReview }},
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

func TestAgentPermissionsSeparateSourceWritesFromDisposableToolOutput(t *testing.T) {
	scope := agentTestScope(t)
	scratch := filepath.Join(t.TempDir(), "tool-output")
	profile := agentPermissionProfile(scope, scratch)
	files := profile["filesystem"].(map[string]any)
	want := map[string]any{":root": "deny", ":minimal": "read",
		filepath.Join(scope.Root, "contract.txt"): "read",
		filepath.Join(scope.Root, "source.go"):    "read", scratch: "write"}
	for file, access := range want {
		if files[file] != access {
			t.Fatalf("required source, runtime or disposable access changed: %s=%v", file, files[file])
		}
	}
	for file, access := range files {
		if access == "write" && file != scratch {
			t.Fatalf("a runtime or other ungranted path became writable: %s", file)
		}
	}
	if profile["network"].(map[string]bool)["enabled"] {
		t.Fatal("native agent acquired network access")
	}
	if _, granted := files[scope.Root]; granted {
		t.Fatal("the entire workspace became writable")
	}
	if !reflect.DeepEqual(agentPermissionFlags(scope, scratch), agentPermissionFlags(scope, scratch)) {
		t.Fatal("native permission provenance depends on map iteration")
	}
	scope.Role, scope.WriteFiles = codingagent.Review, nil
	scope.ReadFiles = []string{"source.go", "contract.txt"}
	files = agentPermissionProfile(scope, scratch)["filesystem"].(map[string]any)
	if files[filepath.Join(scope.Root, "source.go")] != "read" || files[scratch] != "write" {
		t.Fatal("review changed source authority instead of isolating tool output")
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
		t.Fatal("verification commands inherited host storage or unbounded Go workers")
	}
	for _, key := range []string{"OMNI_AGENT_SERVICE_SECRET", "DATABASE_URL", "REDIS_PASSWORD", "CODEX_HOME"} {
		if _, found := environment[key]; found {
			t.Fatalf("tool environment exposes %s", key)
		}
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

func TestAgentNativeReceiptsRejectForeignIncompleteAndUngrantableEvents(t *testing.T) {
	for _, raw := range []string{
		`{"method":"item/completed","params":{"threadId":"foreign","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"{\"questions\":[]}"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"foreign","item":{"type":"commandExecution"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"type":"mcpToolCall"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"interrupted"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"foreign","status":"completed"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`,
	} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		if finished, err := collectAgentFrame(decodeAgentTestFrame(t, raw), []byte(raw), &result); err == nil || finished || result.Completed {
			t.Fatalf("foreign, incomplete, or ungranted receipt accepted: %s", raw)
		}
	}
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	answer := `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"{\"questions\":[]}"}}}`
	if finished, err := collectAgentFrame(decodeAgentTestFrame(t, answer), []byte(answer), &result); err != nil || finished || result.Completed {
		t.Fatalf("a final message alone became completion: finished=%v error=%v", finished, err)
	}
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, answer), []byte(answer), &result); err == nil {
		t.Fatal("duplicate structured answers were accepted")
	}
	completed := `{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`
	if finished, err := collectAgentFrame(decodeAgentTestFrame(t, completed), []byte(completed), &result); err != nil || !finished || !result.Completed {
		t.Fatalf("exact native completion rejected: finished=%v error=%v", finished, err)
	}
}

func TestAgentItemLifecycleRejectsWriteAttemptsAndUnboundCompletion(t *testing.T) {
	start := decodeAgentTestFrame(t, `{"method":"item/started","params":{"item":{"id":"item-1","type":"commandExecution"}}}`)
	complete := decodeAgentTestFrame(t, `{"method":"item/completed","params":{"item":{"id":"item-1","type":"commandExecution","status":"completed"}}}`)
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
	unfinished := decodeAgentTestFrame(t, `{"method":"item/completed","params":{"item":{"id":"item-1","type":"commandExecution","status":"inProgress"}}}`)
	if err := validateAgentItems(unfinished, codingagent.Execute, open); err == nil {
		t.Fatal("an unfinished command became completed evidence")
	}
	if err := validateAgentItems(complete, codingagent.Execute, open); err != nil || len(open) != 0 {
		t.Fatalf("completed command not accounted for: %v", err)
	}
	write := decodeAgentTestFrame(t, `{"method":"item/started","params":{"item":{"id":"edit-1","type":"fileChange"}}}`)
	if err := validateAgentItems(write, codingagent.Review, open); err == nil {
		t.Fatal("independent review acquired source write authority")
	}
}

func TestAgentReceiptPersistenceMustSucceedBeforeEvidenceIsAccepted(t *testing.T) {
	result := codingagent.Result{}
	raw := json.RawMessage(`{"original":"receipt"}`)
	failure := errors.New("durable evidence unavailable")
	if err := recordAgentFrame(&result, raw, func(json.RawMessage) error { return failure }); !errors.Is(err, failure) || len(result.Evidence) != 0 {
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

type agentTestFileWriter func(context.Context, codingagent.FileWrite) error

func (writer agentTestFileWriter) WriteFile(ctx context.Context, edit codingagent.FileWrite) error {
	return writer(ctx, edit)
}

func TestAgentFileWriteRejectsMalformedArgumentsBeforeSideEffects(t *testing.T) {
	for _, arguments := range []string{
		`null`, `[]`, `{}`,
		`{"path":"source.go","expected_content":"original"}`,
		`{"path":"source.go","content":"replacement"}`,
		`{"content":"replacement","expected_content":"original"}`,
		`{"path":"source.go","content":null,"expected_content":"original"}`,
		`{"path":"source.go","content":"replacement","expected_content":null}`,
		`{"path":null,"content":"replacement","expected_content":"original"}`,
		`{"path":"source.go","content":1,"expected_content":"original"}`,
		`{"path":"source.go","content":"replacement","expected_content":false}`,
		`{"path":{},"content":"replacement","expected_content":"original"}`,
		`{"path":"source.go","content":"replacement","content":"","expected_content":"original"}`,
		`{"path":"source.go","content":"replacement","expected_content":"original","expected_content":""}`,
		`{"Path":"source.go","content":"replacement","expected_content":"original"}`,
		`{"path":"source.go","content":"replacement","expected_content":"original","extra":true}`,
	} {
		t.Run(arguments, func(t *testing.T) {
			input := &agentTestToolInput{}
			connection := &connection{input: input, lastFrame: json.RawMessage(`{"original":"request"}`)}
			params, err := json.Marshal(map[string]any{"threadId": "thread", "turnId": "turn", "callId": "call", "tool": "omnidex_write_file", "arguments": json.RawMessage(arguments)})
			if err != nil {
				t.Fatal(err)
			}
			id, writes := 1, 0
			writer := agentTestFileWriter(func(context.Context, codingagent.FileWrite) error { writes++; return nil })
			result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
			if err := connection.handleAgentTool(context.Background(), rpcFrame{ID: &id, Method: "item/tool/call", Params: params}, &result, codingagent.Assignment{Role: codingagent.Execute}, writer, nil); err != nil {
				t.Fatal(err)
			}
			var reply struct {
				Result struct {
					Success bool `json:"success"`
				} `json:"result"`
			}
			if err := json.Unmarshal(input.Bytes(), &reply); err != nil {
				t.Fatal(err)
			}
			if writes != 0 || reply.Result.Success {
				t.Fatal("malformed native arguments reached the writer or received success")
			}
		})
	}
	edit, err := decodeAgentFileWrite(json.RawMessage(`{"path":"source.go","content":"","expected_content":""}`))
	if err != nil || edit.Path != "source.go" || edit.Content != "" || edit.ExpectedContent != "" {
		t.Fatalf("intentional empty string contents were rejected: edit=%+v error=%v", edit, err)
	}
	if _, err := decodeAgentFileWrite(json.RawMessage(`{"path":"source.go","content":"","expected_content":""} {}`)); err == nil {
		t.Fatal("trailing native arguments were accepted")
	}
}
