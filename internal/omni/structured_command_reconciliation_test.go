package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredCommandDecisionCompletesNotesAppAfterLedgerShrinksAcrossWrites(t *testing.T) {
	workspace := t.TempDir()
	commands := []string{
		`mkdir -p src/hooks && printf '%s\n' "import React, { createContext, useState } from 'react';
const NotesContext = createContext();
export const NotesProvider = ({ children }) => {
  const [notes, setNotes] = useState([]);
  const addNote = (note) => setNotes([...notes, note]);
  const deleteNote = (id) => setNotes(notes.filter(note => note.id !== id));
  return <NotesContext.Provider value={{ notes, addNote, deleteNote }}>{children}</NotesContext.Provider>;
};
export default NotesContext;" > src/hooks/useNotes.js`,
		`mkdir -p src && printf '%s\n' "import React from 'react';
import { NotesProvider } from './hooks/useNotes';
import NotesList from './components/NotesList';
export default function App() {
  return <NotesProvider><NotesList /></NotesProvider>;
}" > src/App.js`,
		`mkdir -p src/components && printf '%s\n' "import React from 'react';
import NotesContext from '../hooks/useNotes';
export default function NotesList() {
  return <div>NotesList</div>;
}" > src/components/NotesList.js`,
		`printf '%s\n' "// addNote deleteNote implementation verified" >> src/hooks/useNotes.js`,
		`test -s src/hooks/useNotes.js && test -s src/App.js && test -s src/components/NotesList.js && grep -q addNote src/hooks/useNotes.js && grep -q deleteNote src/hooks/useNotes.js && grep -q NotesProvider src/App.js && grep -q NotesList src/components/NotesList.js && printf 'notes app verified\n'`,
	}
	responses := make([]string, 0, len(commands)+1)
	for _, command := range commands {
		payload, err := json.Marshal(StructuredCommandPayload{Command: command, Done: false})
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, string(payload))
	}
	responses = append(responses, `{"command":"","done":true,"answer":"Notes app context, App.js integration, NotesList, and add/delete functions are verified."}`)
	client := &fakeCommandDecisionClient{responses: responses}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		UserOperation: userOperationModifyExisting,
		ObjectiveLedger: []StructuredObjective{
			{ID: "setup_notes_context", Description: "Set up Notes Context", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "update_appjs_with_notescontext", Description: "Update App.js with NotesContext", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "create_noteslist_component", Description: "Create NotesList component", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "implement_add_and_delete_note_functions", Description: "Implement add and delete note functions", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"continue setting up this project as a React notes app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
		},
	)
	if err != nil {
		t.Fatalf("%v observations=%#v work_items=%#v", err, result.Observations, result.WorkItems)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
	if result.Answer == "" || !strings.Contains(result.Answer, "verified") {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !structuredEventsContain(events, "partial_completion_accepted") {
		t.Fatalf("missing partial completion event: %#v", events)
	}
	if client.calls != len(responses) {
		t.Fatalf("client calls = %d, want %d", client.calls, len(responses))
	}
}

func TestRepeatedFailedCommandExecutesPermissiveRetry(t *testing.T) {
	workspace := t.TempDir()
	command := "ls /omni-nxt-install-failed"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"printf 'alternate path\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"alternate path"}`,
	}}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue", nil, client, &strings.Builder{}, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) < 2 || !strings.Contains(result.Observations[1].Command, "omni-nxt-install-failed") || result.Observations[1].ExitCode != 2 {
		t.Fatalf("repeated failed command should execute as a retry under permissive policy: %#v", result.Observations)
	}
}

func TestDoneTrueWithNonEmptyDockerCommandExecutesBeforeCompletionValidation(t *testing.T) {
	workspace := t.TempDir()
	command := "test -d . && printf 'docker build ok\\ndocker run ok\\nDOCKER_SMOKE_OK running=true restarting=false restart_count=0\\nDOCKER_LOGS_CLEAR\\n'"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":true,"answer":"docker complete"}`,
		`{"command":"","done":true,"answer":"docker lifecycle verified"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "build_docker_image", Description: "Build Docker image", Status: "pending", Required: true},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{
		{Done: false, Reason: "command evidence should be gathered first"},
		{Done: true, Reason: "docker build and run evidence observed", ObjectiveLedger: []StructuredObjective{
			{ID: "build_docker_image", Description: "Build Docker image", Status: "satisfied", Required: true},
		}},
	}}
	stdout := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "Build and run the Docker image.", nil, client, stdout, &strings.Builder{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
		CompletionChecker:       checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "DOCKER_SMOKE_OK") {
		t.Fatalf("non-empty done=true command was not executed, stdout=%q observations=%#v", stdout.String(), result.Observations)
	}
	if !structuredEventsContain(events, "structured_done_ignored") {
		t.Fatalf("done=true command should emit done-ignored execution event: %#v", events)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestRepeatedFailedCommandDoesNotHardForceShellSpecialistRecovery(t *testing.T) {
	workspace := t.TempDir()
	command := "false"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"printf 'alternate path\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"alternate path"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "test -d .",
		Rationale: "Use a different command after the planner repeated a blocked failure.",
	}}}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
			Done:   true,
			Reason: "alternate command recovered progress",
		}}},
	})
	if err != nil {
		t.Fatalf("%v observations=%#v shell_inputs=%#v stdout=%q", err, result.Observations, shell.inputs, stdout.String())
	}
	if len(shell.inputs) != 0 {
		t.Fatalf("repeated command should not hard-force shell specialist under permissive retry policy: %#v", shell.inputs)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestWriteRecoveryBypassesShellAfterRepeatedInvalidSpecialistProposals(t *testing.T) {
	decision := ProgressionDecision{
		Reason:           "workspace inspection has not produced app files; creation step is now required",
		RecoveryToolTask: "Required next behavior: create or modify the actual project files now. Do not continue with read-only inventory commands.",
	}
	result := &CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Step: 1, RejectedCommand: "touch README.md", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive file content or verification; placeholder-only command \"touch README.md\" does not satisfy it"},
		{Step: 2, RejectedCommand: "touch index.zig", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive file content or verification; placeholder-only command \"touch index.zig\" does not satisfy it"},
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command: "touch src/main.zig",
	}}}
	events := []StructuredCommandEvent{}
	handled, err := runProgressionGateRecovery(context.Background(), 3, "Build a Rust CLI calculator", decision, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: t.TempDir(),
		ShellSpecialist:         shell,
	}, WorksiteSurvey{}, &strings.Builder{}, &strings.Builder{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, result)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("expected recovery to fall through to planner after repeated invalid shell proposals")
	}
	if len(shell.inputs) != 0 {
		t.Fatalf("shell specialist should be bypassed: %#v", shell.inputs)
	}
	if !structuredEventsContain(events, "progression_gate_shell_bypassed") {
		t.Fatalf("missing shell bypass event: %#v", events)
	}
}

func TestWriteRecoveryBypassesShellAfterRepeatedDocumentationDownloadProposals(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Step: 1, RejectedCommand: "curl -s https://ziglang.org/documentation/master/ > zig_doc.html", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive source/build/test files; documentation download command does not satisfy it"},
		{Step: 2, RejectedCommand: "curl -s https://ziglang.org/documentation/master/ > zig_doc.html", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive source/build/test files; documentation download command does not satisfy it"},
	}
	if !shouldBypassShellSpecialistForWriteRecovery("Required next behavior: create or modify the actual project files now with substantive source/build/test files.", observations) {
		t.Fatal("expected repeated documentation-download proposals to bypass shell specialist")
	}
}

func TestPlannerAcceptsSubstantiveWriteThenContinuesObjectives(t *testing.T) {
	workspace := t.TempDir()
	contract := buildImplementationArchitectContract(
		"please continue setting up this project as a react js note app",
		"",
		workspace,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		nil,
	)
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json", "vite.config.js", "index.html", "src/main.jsx", "scripts/smoke-test.mjs")
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.js"), []byte(deterministicGenericReactApp(contract)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.css"), []byte(deterministicGenericReactAppCSS(contract)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "src", "components"), 0o755); err != nil {
		t.Fatal(err)
	}
	substantive := "cat > src/components/NoteManager.js <<'EOF'\nexport default function NoteManager(){ return 'notes crud memory'; }\nEOF"
	crudWrite := "printf '\\n// crud operations implemented\\n' >> src/components/NoteManager.js"
	memoryWrite := "printf '\\n// memory storage implemented\\n' >> src/components/NoteManager.js"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForTest(substantive) + `,"done":false,"answer":"","objective_ledger":[{"id":"create_note_manager_component","description":"Create notes component structure","status":"pending","source":"user_explicit","required":true},{"id":"implement_crud_operations","description":"Implement CRUD operations","status":"pending","source":"user_explicit","required":true},{"id":"store_notes_in_memory","description":"Store notes in memory","status":"pending","source":"user_explicit","required":true}]}`,
		`{"command":` + quoteJSONForTest(crudWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"implement_crud_operations","description":"Implement CRUD operations","status":"satisfied","evidence":"NoteManager contains CRUD marker"}]}`,
		`{"command":` + quoteJSONForTest(memoryWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"store_notes_in_memory","description":"Store notes in memory","status":"satisfied","evidence":"NoteManager contains memory marker"}]}`,
		`{"command":"grep -q 'memory storage implemented' src/components/NoteManager.js","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Notes component created."}`,
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"please continue setting up this project as a react js note app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	if err != nil {
		t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
	}
	if structuredEventsContain(events, "structured_planner_repair_started") {
		t.Fatalf("substantive scaffold should execute as setup progress, not enter repair: %#v", events)
	}
	if !structuredEventsContain(events, "partial_completion_accepted") {
		t.Fatalf("missing partial completion continuation after substantive scaffold: %#v", events)
	}
	if structuredEventsContain(events, "structured_command_rejected") {
		t.Fatalf("substantive scaffold should not be rejected: %#v", events)
	}
	if len(result.Observations) < 1 || !strings.Contains(result.Observations[0].Command, "cat > src/components/NoteManager.js") {
		t.Fatalf("expected substantive write observation first: %#v", result.Observations)
	}
	if _, err := os.Stat(filepath.Join(workspace, "src/components/NoteManager.js")); err != nil {
		t.Fatalf("expected component file: %v", err)
	}
}

func TestProposeValidatedShellCommandRepairsDependencyDriftWithDirectFeedback(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"react":"latest","react-dom":"latest"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	toolTask := "create the notes UI component and in-memory CRUD behavior using existing React dependencies"
	writeComponent := "cat > src/NoteManager.jsx <<'EOF'\nexport default function NoteManager(){ return 'notes crud memory' }\nEOF"
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "npm install react-router-dom", Rationale: "Add routing."},
		{Command: writeComponent, Rationale: "Write the requested component using existing dependencies."},
	}}
	result := &CommandDecisionResult{}
	proposal, ok, err := proposeValidatedShellCommand(
		context.Background(),
		3,
		"continue setting up this existing React project as a note app",
		toolTask,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace, ShellSpecialist: shell},
		WorksiteSurvey{PackageManager: packageManagerNPM},
		&[]StructuredObjective{},
		nil,
		func(ctx context.Context, question string) (string, error) { return "no", nil },
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected repaired shell proposal, observations=%#v inputs=%#v", result.Observations, shell.inputs)
	}
	if !strings.Contains(proposal.Command, "NoteManager.jsx") {
		t.Fatalf("proposal = %#v observations=%#v inputs=%#v", proposal, result.Observations, shell.inputs)
	}
	if len(shell.inputs) < 2 || shell.inputs[1].RepairFeedback == "" || shell.inputs[1].RejectedCommand == "" {
		t.Fatalf("repair feedback not forwarded: %#v", shell.inputs)
	}
}

func TestShellSpecialistRepairsRejectedDependencyScopeDriftLocally(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"react":"latest","react-dom":"latest"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"create the notes UI component and in-memory CRUD behavior using existing React dependencies"}`,
		`{"command":"","done":true,"answer":"Notes UI created."}`,
	}}
	writeComponent := "cat > src/NoteManager.jsx <<'EOF'\nexport default function NoteManager(){ return 'notes crud memory' }\nEOF"
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "npm install react-router-dom", Rationale: "Add routing."},
		{Command: writeComponent, Rationale: "Write the requested component using existing dependencies."},
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"continue setting up this existing React project as a note app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		func(ctx context.Context, question string) (string, error) { return "no", nil },
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			ShellSpecialist:         shell,
			CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{
				{Done: true, Reason: "notes component written"},
			}},
		},
	)
	if err != nil {
		var exhausted CommandDecisionExhaustedError
		if !errors.As(err, &exhausted) {
			t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
		}
	}
	if len(shell.inputs) < 2 {
		t.Fatalf("shell specialist calls = %d, want at least initial + repair call; inputs=%#v obs=%#v", len(shell.inputs), shell.inputs, result.Observations)
	}
	if shell.inputs[1].RepairAttempt != 1 {
		t.Fatalf("second shell call should be repair attempt 1: %#v", shell.inputs[1])
	}
	if shell.inputs[1].RepairFeedback == "" {
		t.Fatalf("second shell call missing repair feedback: %#v", shell.inputs[1])
	}
	if shell.inputs[1].RejectedCommand == "" {
		t.Fatalf("second shell call missing rejected command preview: %#v", shell.inputs[1])
	}
	if len(shell.inputs[1].Observations) == 0 || !observationsContainStderr(shell.inputs[1].Observations, "dependency scope drift") {
		t.Fatalf("repair input missing validator feedback: %#v", shell.inputs[1].Observations)
	}
	if !structuredEventsContain(events, "structured_tool_delegation_repair_started") || !structuredEventsContain(events, "structured_tool_delegation_repair_accepted") {
		t.Fatalf("missing shell repair events: %#v", events)
	}
	if !observationsContainCommand(result.Observations, writeComponent) {
		t.Fatalf("expected repaired write command in observations: %#v", result.Observations)
	}
	if _, err := os.Stat(filepath.Join(workspace, "src/NoteManager.jsx")); err != nil {
		t.Fatalf("expected notes component: %v", err)
	}
}

func TestShellSpecialistStopsLocalRepairAfterRepeatedRejectedCommand(t *testing.T) {
	result := &CommandDecisionResult{}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "npm install react-router-dom", Rationale: "Add routing."},
		{Command: "npm install react-router-dom", Rationale: "Retry routing."},
		{Command: "cat > src/NoteManager.jsx <<'EOF'\nexport default function NoteManager(){ return 'notes'; }\nEOF", Rationale: "Too late."},
	}}
	events := []StructuredCommandEvent{}
	_, ok, err := proposeValidatedShellCommand(
		context.Background(),
		4,
		"continue notes app",
		"create the notes UI component and in-memory CRUD behavior using existing React dependencies",
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: t.TempDir(), ShellSpecialist: shell},
		WorksiteSurvey{},
		&[]StructuredObjective{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		func(ctx context.Context, question string) (string, error) { return "no", nil },
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected repeated rejected proposal to stop local shell repair")
	}
	if len(shell.inputs) != 2 {
		t.Fatalf("shell calls = %d, want stop after repeated rejection", len(shell.inputs))
	}
	if shell.inputs[0].RepairAttempt != 0 || shell.inputs[1].RepairAttempt != 1 {
		t.Fatalf("repair attempts = %#v", shell.inputs)
	}
	if !structuredEventsContain(events, "structured_tool_delegation_repair_repeated") {
		t.Fatalf("missing repeated repair event: %#v", events)
	}
}

func TestShellSpecialistRequestRaisesTemperatureOnlyForRepairAttempt(t *testing.T) {
	initial := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{ToolTask: "inspect"})
	if got := initial.Options["temperature"]; got != 0 {
		t.Fatalf("initial temperature = %#v, want 0", got)
	}
	repair := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		ToolTask:        "inspect",
		RepairAttempt:   1,
		RepairFeedback:  "shell specialist command rejected: placeholder-only",
		RejectedCommand: "touch src/App.css",
	})
	if got := repair.Options["temperature"]; got != defaultShellSpecialistRepairTemperature {
		t.Fatalf("repair temperature = %#v, want %#v", got, defaultShellSpecialistRepairTemperature)
	}
	if !strings.Contains(repair.Messages[1].Content, `"repair_attempt":1`) {
		t.Fatalf("repair prompt missing repair_attempt: %s", repair.Messages[1].Content)
	}
	if len(repair.Messages) != 4 {
		t.Fatalf("repair message count = %d, want 4", len(repair.Messages))
	}
	if repair.Messages[2].Role != "assistant" || !strings.Contains(repair.Messages[3].Content, "repair_feedback") {
		t.Fatalf("missing shell repair follow-up: %#v", repair.Messages[2:])
	}
}

func TestCodeContentSpecialistRequestIncludesRepairFeedbackOnRetry(t *testing.T) {
	item := ArchitectWorkItem{ID: "style_react_app", Operation: "create", Path: "src/App.css"}
	initial := buildCodeContentSpecialistRequest(CodeContentSpecialistInput{WorkItem: item})
	if got := initial.Options["temperature"]; got != 0 {
		t.Fatalf("initial temperature = %#v, want 0", got)
	}
	if len(initial.Messages) != 2 {
		t.Fatalf("initial message count = %d, want 2", len(initial.Messages))
	}
	repair := buildCodeContentSpecialistRequest(CodeContentSpecialistInput{
		WorkItem:        item,
		RepairAttempt:   1,
		RepairFeedback:  "code content specialist content rejected: architect work item content kind rejected: .css path requires CSS, not JavaScript or React source; rewrite the file to match file_contract.language exactly",
		RejectedContent: "export default function App() { return null; }",
	})
	if got := repair.Options["temperature"]; got != defaultShellSpecialistRepairTemperature {
		t.Fatalf("repair temperature = %#v, want %#v", got, defaultShellSpecialistRepairTemperature)
	}
	if !strings.Contains(repair.Messages[1].Content, `"repair_attempt":1`) {
		t.Fatalf("repair prompt missing repair_attempt: %s", repair.Messages[1].Content)
	}
	if !strings.Contains(repair.Messages[1].Content, "repair_feedback") {
		t.Fatalf("repair prompt missing repair_feedback: %s", repair.Messages[1].Content)
	}
	if len(repair.Messages) != 4 {
		t.Fatalf("repair message count = %d, want 4 (system, initial user, rejected assistant, repair user)", len(repair.Messages))
	}
	if repair.Messages[2].Role != "assistant" {
		t.Fatalf("third message role = %q, want assistant", repair.Messages[2].Role)
	}
	if !strings.Contains(repair.Messages[3].Content, "repair_feedback") {
		t.Fatalf("repair follow-up missing repair_feedback: %s", repair.Messages[3].Content)
	}
}

func TestLatestCodeContentRepairFeedback(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Stderr: "shell specialist command rejected: unrelated"},
		{Stderr: "code content specialist content rejected: architect work item content kind rejected: .css path requires CSS, not JavaScript or React source; rewrite the file"},
	}
	if got := latestCodeContentRepairFeedback(observations); !strings.Contains(got, "content kind rejected") {
		t.Fatalf("latestCodeContentRepairFeedback() = %q", got)
	}
}

func TestRepeatedSuccessfulCommandSkipsAndUsesCompletedEvidence(t *testing.T) {
	workspace := t.TempDir()
	command := "ls -la " + workspace
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"inspected package"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "test -d .",
		Rationale: "Use prior ls output and inspect a new target.",
	}}}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "inspect workspace and verify it exists", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{
			{Done: false, Reason: "objectives still pending", ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			}},
			{Done: false, Reason: "use completed ls evidence first", ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			}},
			{Done: true, Reason: "recovery inspected next target", ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "ls output"},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "test -d ."},
			}},
		}},
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	successCount := 0
	skipCount := 0
	for _, obs := range result.Observations {
		if obs.Command == command && obs.ExitCode == 0 {
			successCount++
		}
		if strings.HasPrefix(obs.Command, "SKIPPED_REPEAT_SUCCESS:") && obs.RejectedCommand == command {
			skipCount++
		}
	}
	if successCount != 1 || skipCount != 1 {
		t.Fatalf("expected repeated successful command to execute once and skip once, success=%d skip=%d observations=%#v", successCount, skipCount, result.Observations)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestBlockedFalseDoneForcesRecoveryBeforeNormalPlanning(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"cat ` + filepath.ToSlash(filepath.Join(workspace, "index.html")) + `","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"","done":true,"answer":"recovered"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "test -d .",
		Rationale: "Recover from missing index.html by discovering files.",
	}}}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "inspect project structure", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
			Done:   true,
			Reason: "missing-file recovery discovered project structure",
			ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_project_structure", Description: "Inspect project structure", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "discovered project structure"},
			},
		}}},
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{{ID: "inspect_project_structure", Description: "Inspect project structure", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shell.inputs) != 1 {
		t.Fatalf("shell specialist calls = %d, want 1", len(shell.inputs))
	}
	if !strings.Contains(shell.inputs[0].ToolTask, "target path does not exist") {
		t.Fatalf("missing-file recovery task = %q", shell.inputs[0].ToolTask)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestDelegatedShellExecutionFailureRepairsWithResponsibleShellSpecialist(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.js"), []byte("export default function App(){ return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"inspect existing React source files before editing"}`,
		`{"command":"","done":true,"answer":"source inspected"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "cat src/components/ActualButMissing.js", Rationale: "Inspect suspected component."},
		{Command: "cat src/components/ActualButMissing.js", Rationale: "Retry suspected component."},
		{Command: "find src -maxdepth 3 -type f", Rationale: "Discover actual source files after missing path feedback."},
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"inspect this React app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			ShellSpecialist:         shell,
			CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
				Done:   true,
				Reason: "bounded source discovery ran after missing-file feedback",
			}}},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
	}
	if len(shell.inputs) != 3 {
		t.Fatalf("shell calls = %d, want initial plus local repair attempts", len(shell.inputs))
	}
	if !observationsContainStderr(shell.inputs[1].Observations, "No such file or directory") {
		t.Fatalf("responsible shell specialist did not receive execution failure: %#v", shell.inputs[1].Observations)
	}
	if !observationsContainStderr(shell.inputs[2].Observations, "repeats the latest failed execution") {
		t.Fatalf("responsible shell specialist did not receive repeat rejection: %#v", shell.inputs[2].Observations)
	}
	if result.Command != "find src -maxdepth 3 -type f" {
		t.Fatalf("final command = %q", result.Command)
	}
	if !structuredEventsContain(events, "structured_command_rejected") {
		t.Fatalf("missing direct shell rejection event: %#v", events)
	}
}

func TestValidateShellProposalRejectsMissingFileRecoveryInvalidReadRepeat(t *testing.T) {
	toolTask := "Recovery required. A read/inspect command failed because the target path does not exist. Invalid command: cat src/components/ActualButMissing.js. Failure: step=18 command=cat src/components/ActualButMissing.js exit_code=1 stderr=cat: src/components/ActualButMissing.js: No such file or directory Required next behavior: inspect the parent directory, run a bounded file discovery command, inspect package.json if present, update the workspace model, then continue with discovered files. Do not retry the invalid path unless new evidence proves it exists."
	err := validateShellProposalAgainstToolTask("cat src/components/ActualButMissing.js", toolTask)
	if err == nil || !strings.Contains(err.Error(), "must not retry invalid read command") {
		t.Fatalf("expected invalid read retry rejection, got %v", err)
	}
	if err := validateShellProposalAgainstToolTask("find src -maxdepth 3 -type f", toolTask); err != nil {
		t.Fatalf("bounded discovery should pass missing-file recovery validation: %v", err)
	}
}

func TestValidateStructuredCommandProtectsActiveWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "test_project_20260520115716")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		fmt.Sprintf("rm -r %q", projectDir),
		fmt.Sprintf("rmdir %q", projectDir),
		fmt.Sprintf("mv %q %q", projectDir, filepath.Join(root, "moved")),
		fmt.Sprintf("rm %q && mkdir %q", filepath.Join(root, "scratch"), filepath.Join(root, "scratch")),
	} {
		err := validateStructuredCommandForRun(command, nil, projectDir, nil)
		if err == nil {
			t.Fatalf("command %q should be rejected", command)
		}
	}
	if err := validateStructuredCommandForRun("mkdir -p . && npm init -y", nil, projectDir, nil); err != nil {
		t.Fatalf("additive initialization should be allowed: %v", err)
	}
}
