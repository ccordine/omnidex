package codex

import (
	"bufio"
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

func TestNativeUserInputRequestUsesDurableQuestionContract(t *testing.T) {
	id := 9
	frame := rpcFrame{ID: &id, Method: "item/tool/requestUserInput", Params: json.RawMessage(`{
		"threadId":"thread","turnId":"turn","itemId":"question-item","isBlocking":true,"autoResolutionMs":null,
		"questions":[
			{"header":"Toolchain","id":"toolchain","question":"Which toolchain policy should apply?","options":[{"label":"Local","description":"Use the installed compiler offline."},{"label":"Exact","description":"Require Go 1.25 itself."}]},
			{"header":"Scope","id":"scope","question":"Should documentation be included?","options":null}
		]
	}`)}
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	questions, handled, err := mapAgentUserInputRequest(codingagent.Assignment{Role: codingagent.Plan}, frame, &result)
	if err != nil || !handled || !reflect.DeepEqual(questions, []string{
		"Which toolchain policy should apply? Options: Local — Use the installed compiler offline.; Exact — Require Go 1.25 itself.",
		"Should documentation be included?",
	}) {
		t.Fatalf("native questions=%#v handled=%v error=%v", questions, handled, err)
	}
	content, err := agentQuestionResult(codingagent.Plan, questions)
	if err != nil {
		t.Fatal(err)
	}
	var workload codingagent.Workload
	if err := json.Unmarshal(content, &workload); err != nil || len(workload.Tasks) != 0 || len(workload.Checks) != 0 || !reflect.DeepEqual(workload.Questions, questions) {
		t.Fatalf("planning question result=%s error=%v", content, err)
	}

	for _, mutation := range []func(*rpcFrame){
		func(frame *rpcFrame) {
			frame.Params = json.RawMessage(strings.ReplaceAll(string(frame.Params), `"thread"`, `"foreign"`))
		},
		func(frame *rpcFrame) {
			frame.Params = json.RawMessage(strings.ReplaceAll(string(frame.Params), `"isBlocking":true`, `"isBlocking":false`))
		},
		func(frame *rpcFrame) {
			frame.Params = json.RawMessage(strings.ReplaceAll(string(frame.Params), `"id":"scope"`, `"id":"toolchain"`))
		},
		func(frame *rpcFrame) {
			frame.Params = json.RawMessage(strings.ReplaceAll(string(frame.Params), `"isBlocking":true`, `"isBlocking":true,"extra":1`))
		},
		func(frame *rpcFrame) {
			frame.Params = json.RawMessage(strings.ReplaceAll(string(frame.Params), `"header":"Scope"`, `"header":"Scope","isSecret":true`))
		},
	} {
		invalid := frame
		mutation(&invalid)
		if _, handled, err := mapAgentUserInputRequest(codingagent.Assignment{Role: codingagent.Plan}, invalid, &result); !handled || err == nil {
			t.Fatalf("invalid native question accepted: %s", invalid.Params)
		}
	}
}

func TestAgentTransportAllowsOnlyBoundToolsAndUserQuestions(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		method  string
		wantErr bool
	}{
		{name: "scoped file tool", method: "item/tool/call"},
		{name: "durable user question", method: "item/tool/requestUserInput"},
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
		`{"method":"item/completed","params":{"threadId":"foreign","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"{\"questions\":[]}"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"foreign","item":{"type":"commandExecution"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"type":"mcpToolCall"}}}`,
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
	answer := `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"{\"questions\":[]}"}}}`
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

func TestAgentNativePlanAndReviewItemsAreAuthoritativeResults(t *testing.T) {
	plan := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	planItem := `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"plan","type":"plan","status":"completed","text":"{\"tasks\":[]}"}}}`
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, planItem), []byte(planItem), &plan, codingagent.Plan); err != nil || string(plan.Content) != `{"tasks":[]}` {
		t.Fatalf("native plan result lost: content=%s error=%v", plan.Content, err)
	}

	review := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	reviewItem := `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"review","type":"exitedReviewMode","status":"completed","review":"{\"findings\":[],\"questions\":[]}\nFull review comments: detail"}}}`
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, reviewItem), []byte(reviewItem), &review, codingagent.Review); err != nil {
		t.Fatal(err)
	}
	var text string
	if err := json.Unmarshal(review.Content, &text); err != nil || !strings.Contains(text, "Full review comments") {
		t.Fatalf("native review text lost: content=%s error=%v", review.Content, err)
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
	err := validateAgentItems(write, codingagent.Review, open)
	if err == nil {
		t.Fatal("independent review acquired source write authority")
	}
	var retryable interface{ Retryable() bool }
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Fatalf("blocked reviewer edit did not permit a fresh bounded review: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("blocked reviewer edit changed native item authority: %v", open)
	}
}

func TestNativeReviewExitClosesItsMessagePlaceholder(t *testing.T) {
	open := map[string]string{}
	frames := []string{
		`{"method":"item/started","params":{"item":{"id":"message","type":"agentMessage"}}}`,
		`{"method":"item/started","params":{"item":{"id":"review","type":"exitedReviewMode"}}}`,
		`{"method":"item/completed","params":{"item":{"id":"review","type":"exitedReviewMode","status":"completed"}}}`,
	}
	for _, raw := range frames {
		if err := validateAgentItems(decodeAgentTestFrame(t, raw), codingagent.Review, open); err != nil {
			t.Fatal(err)
		}
	}
	if len(open) != 0 {
		t.Fatalf("native review placeholder remained open: %v", open)
	}

	// Native review commentary is delivered as complete message starts. The
	// server does not send item/completed for those messages before it starts
	// the final-answer placeholder whose body is carried by exitedReviewMode.
	// Both the ordinary and overall-review roles must account for that observed
	// lifecycle without treating an unrelated message phase as completed.
	for _, fixture := range []struct {
		name string
		role codingagent.Role
		ids  []string
	}{
		{name: "review commentary", role: codingagent.Review, ids: []string{"progress", "final"}},
		{name: "overall review commentary sequence", role: codingagent.OverallReview, ids: []string{"initial", "followup", "final"}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			open := map[string]string{}
			for index, id := range fixture.ids {
				phase := "commentary"
				if index == len(fixture.ids)-1 {
					phase = "final_answer"
				}
				raw := `{"method":"item/started","params":{"item":{"id":"` + id + `","type":"agentMessage","phase":"` + phase + `"}}}`
				if err := validateAgentItems(decodeAgentTestFrame(t, raw), fixture.role, open); err != nil {
					t.Fatal(err)
				}
			}
			start := `{"method":"item/started","params":{"item":{"id":"review","type":"exitedReviewMode"}}}`
			complete := `{"method":"item/completed","params":{"item":{"id":"review","type":"exitedReviewMode","status":"completed"}}}`
			if err := validateAgentItems(decodeAgentTestFrame(t, start), fixture.role, open); err != nil {
				t.Fatal(err)
			}
			if err := validateAgentItems(decodeAgentTestFrame(t, complete), fixture.role, open); err != nil {
				t.Fatal(err)
			}
			if len(open) != 0 {
				t.Fatalf("native review placeholders remained open: %v", open)
			}
		})
	}

	open = map[string]string{
		"first":  nativeItemState("agentMessage", "final_answer"),
		"second": nativeItemState("agentMessage", "final_answer"),
	}
	start := `{"method":"item/started","params":{"item":{"id":"review","type":"exitedReviewMode"}}}`
	complete := `{"method":"item/completed","params":{"item":{"id":"review","type":"exitedReviewMode","status":"completed"}}}`
	if err := validateAgentItems(decodeAgentTestFrame(t, start), codingagent.Review, open); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentItems(decodeAgentTestFrame(t, complete), codingagent.Review, open); err == nil {
		t.Fatal("native review exit hid multiple incomplete messages")
	}

	open = map[string]string{"unknown": nativeItemState("agentMessage", "custom")}
	if err := validateAgentItems(decodeAgentTestFrame(t, start), codingagent.Review, open); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentItems(decodeAgentTestFrame(t, complete), codingagent.Review, open); err == nil {
		t.Fatal("native review exit hid an unknown unfinished message phase")
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

type agentTestFileWriter func(context.Context, codingagent.FileWrite) error

func (writer agentTestFileWriter) WriteFile(ctx context.Context, edit codingagent.FileWrite) error {
	return writer(ctx, edit)
}

func (writer agentTestFileWriter) DeleteFile(context.Context, codingagent.FileDelete) error {
	return nil
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

func TestAgentFileDeleteRequiresExactPathAndExpectedContent(t *testing.T) {
	for _, arguments := range []string{
		`null`, `[]`, `{}`,
		`{"path":"source.go"}`,
		`{"expected_content":"original"}`,
		`{"path":null,"expected_content":"original"}`,
		`{"path":"source.go","expected_content":null}`,
		`{"path":"source.go","expected_content":1}`,
		`{"path":"source.go","expected_content":"original","extra":true}`,
		`{"path":"source.go","path":"other.go","expected_content":"original"}`,
	} {
		if _, err := decodeAgentFileDelete(json.RawMessage(arguments)); err == nil {
			t.Fatalf("malformed deletion accepted: %s", arguments)
		}
	}
	deletion, err := decodeAgentFileDelete(json.RawMessage(`{"path":"source.go","expected_content":""}`))
	if err != nil || deletion.Path != "source.go" || deletion.ExpectedContent != "" {
		t.Fatalf("valid deletion rejected: deletion=%+v error=%v", deletion, err)
	}
	if _, err := decodeAgentFileDelete(json.RawMessage(`{"path":"source.go","expected_content":""} {}`)); err == nil {
		t.Fatal("trailing deletion arguments were accepted")
	}
}
