package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectWorkItemInvalidIndexHTMLFailsWithoutFallbackOrAdvancing(t *testing.T) {
	workspace := t.TempDir()
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{Content: "<!doctype html><html><body><main>No root</main><script type=\"module\" src=\"/src/main.jsx\"></script></body></html>", Rationale: "missing root"},
		{Content: "<!doctype html><html><body><main>No root</main><script type=\"module\" src=\"/src/main.jsx\"></script></body></html>", Rationale: "same missing root"},
		{Content: "<!doctype html><html><body><main>No root</main><script type=\"module\" src=\"/src/main.jsx\"></script></body></html>", Rationale: "same missing root"},
	}}
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Command: "architect.apply create package.json", ExitCode: 0},
		{Command: "architect.apply create vite.config.js", ExitCode: 0},
	}}
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		workspace,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		result.Observations,
	)
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json", "vite.config.js")
	events := []StructuredCommandEvent{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		8,
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodeContentSpecialist:   code,
		},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err == nil {
		t.Fatal("expected invalid index.html to fail after focused repair attempts")
	}
	if handled {
		t.Fatal("invalid index.html must not be reported as handled")
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "index.html")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid index.html unexpectedly wrote a file: %v", statErr)
	}
	for _, wantEvent := range []string{
		"architect_work_item_repair_started",
		"architect_work_item_repair_rejected",
		"architect_work_item_repair_repeated_content_rejected",
		"architect_work_item_failed_with_evidence",
	} {
		if !structuredEventsContain(events, wantEvent) {
			t.Fatalf("missing %s event: %#v", wantEvent, events)
		}
	}
	if len(code.inputs) < 3 {
		t.Fatalf("expected focused retries for index.html, got inputs=%#v", code.inputs)
	}
	for i := 0; i < 3; i++ {
		if code.inputs[i].WorkItem.Path != "index.html" {
			t.Fatalf("architect advanced before resolving index.html; input %d = %#v", i, code.inputs[i].WorkItem)
		}
	}
	if code.inputs[1].RepairAttempt != 1 {
		t.Fatalf("second index.html attempt missing repair attempt: %#v", code.inputs[1])
	}
	if code.inputs[1].RepairFeedback == "" {
		t.Fatalf("second index.html attempt missing repair feedback: %#v", code.inputs[1])
	}
	if code.inputs[1].RejectedContent == "" {
		t.Fatalf("second index.html attempt missing rejected content preview: %#v", code.inputs[1])
	}
	for _, forbidden := range []string{"architect_work_item_fallback_selected", "architect_work_item_fallback_validated", "architect_work_item_content_fallback_used", "architect_work_item_applied"} {
		if structuredEventsContain(events, forbidden) {
			t.Fatalf("invalid specialist output used forbidden fallback event %s: %#v", forbidden, events)
		}
	}
}

func TestArchitectWorkItemInvalidViteConfigFailsWithoutFallback(t *testing.T) {
	workspace := t.TempDir()
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{Content: "export default {}", Rationale: "missing plugin"},
		{Content: "export default {}", Rationale: "same missing plugin"},
		{Content: "export default {}", Rationale: "same missing plugin"},
	}}
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{{Command: "architect.apply create package.json", ExitCode: 0}}}
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		workspace,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		result.Observations,
	)
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json")
	events := []StructuredCommandEvent{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		9,
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodeContentSpecialist:   code,
		},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err == nil {
		t.Fatal("expected invalid Vite config to fail after focused repair attempts")
	}
	if handled {
		t.Fatal("invalid Vite config must not be reported as handled")
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "vite.config.js")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid Vite config unexpectedly wrote a file: %v", statErr)
	}
	if !structuredEventsContain(events, "architect_work_item_failed_with_evidence") {
		t.Fatalf("missing explicit failure event: %#v", events)
	}
	for _, forbidden := range []string{"architect_work_item_fallback_selected", "architect_work_item_fallback_validated", "architect_work_item_content_fallback_used", "architect_work_item_applied"} {
		if structuredEventsContain(events, forbidden) {
			t.Fatalf("invalid specialist output used forbidden fallback event %s: %#v", forbidden, events)
		}
	}
}

func TestArchitectContentKindRejectsCSSForJavaScriptPath(t *testing.T) {
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		t.TempDir(),
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	item := ArchitectWorkItem{ID: "create_react_entrypoint", Operation: "create", CWD: ".", Path: "src/main.jsx"}
	err := validateCodeContentProposalForArchitectItem("body { color: red; background: white; }\n.app { display: grid; }\n", contract, item)
	if err == nil {
		t.Fatal("expected CSS content to be rejected for JS/JSX path")
	}
	if !isArchitectContentKindValidationError(err) {
		t.Fatalf("expected content-kind rejection, got %v", err)
	}
}

func TestArchitectContentKindRejectsJavaScriptForCSSPath(t *testing.T) {
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		t.TempDir(),
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	item := ArchitectWorkItem{ID: "style_react_app", Operation: "create", CWD: ".", Path: "src/App.css"}
	err := validateCodeContentProposalForArchitectItem("import React from 'react';\nexport default function App() { return null }\n", contract, item)
	if err == nil {
		t.Fatal("expected JavaScript content to be rejected for CSS path")
	}
	if !isArchitectContentKindValidationError(err) {
		t.Fatalf("expected content-kind rejection, got %v", err)
	}
}

func TestPackageMetadataCommandsAllowedForPackageWork(t *testing.T) {
	toolTask := "work_kind: package_metadata_update setup_react_package_metadata configure_package_scripts install_dependencies"
	for _, command := range []string{
		`npm install react react-dom vite @vitejs/plugin-react`,
		`npm pkg set scripts.dev="vite --host 0.0.0.0"`,
		`npm pkg set scripts.build="vite build"`,
		`npm pkg set scripts.preview="vite --host 0.0.0.0"`,
		`npm pkg set type=module`,
		`npm pkg delete scripts.test`,
	} {
		if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
			t.Fatalf("package metadata command %q should be allowed: %v", command, err)
		}
	}
}

func TestPackageMetadataDependencyScopeRejectsUnrequestedDependency(t *testing.T) {
	toolTask := "work_kind: package_metadata_update setup_react_package_metadata install_dependencies"
	err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		toolTask,
		"Add routing because many React apps commonly need navigation.",
	)
	if err == nil {
		t.Fatal("expected react-router-dom to be rejected for package metadata work")
	}
	ledger := []StructuredObjective{{
		ID:       "setup_react_package_metadata",
		Status:   "pending",
		Source:   structuredObjectiveSourceUserExplicit,
		Required: true,
		Packages: reactVitePackageMetadataDependencies(),
	}}
	if err := validateStructuredCommandForRun("npm install react-router-dom", nil, t.TempDir(), ledger); err == nil {
		t.Fatal("dependency scope validation allowed unrequested react-router-dom")
	}
}

func TestSourceFileWorkFailsWithCapabilityEvidenceWhenAllActorsUnavailable(t *testing.T) {
	workspace := t.TempDir()
	prompt := "Build a React notes app"
	toolTask := "Implementation architect target root: . Create or modify the actual project files."
	survey := WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM}
	contract := buildImplementationArchitectContract(prompt, toolTask, workspace, survey, nil)
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json", "vite.config.js", "index.html", "src/main.jsx", "scripts/smoke-test.mjs")
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Command: "architect.apply create package.json", ExitCode: 0},
		{Command: "architect.apply create vite.config.js", ExitCode: 0},
		{Command: "architect.apply create index.html", ExitCode: 0},
		{Command: "architect.apply create src/main.jsx", ExitCode: 0},
		{Command: "architect.apply create scripts/smoke-test.mjs", ExitCode: 0},
	}}
	events := []StructuredCommandEvent{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		7,
		prompt,
		toolTask,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
		survey,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected source work to fail with capability evidence")
	}
	if !structuredEventsContain(events, "architect_work_item_no_capable_actor") {
		t.Fatalf("missing no-capable-actor event: %#v", events)
	}
	if got := result.Observations[len(result.Observations)-1].Stderr; !strings.Contains(got, "no capable actor configured") {
		t.Fatalf("missing capability evidence stderr: %q", got)
	}
}

func TestCursorArchitectAgentOwnsCodingTestingAndValidationDelegation(t *testing.T) {
	workspace := t.TempDir()
	cursor := &fakeCursorArchitectAgent{
		results: []CursorArchitectAgentResult{{Summary: "changed files and validated proofs", AgentID: "agent_1", RunID: "run_1"}},
		run: func(input CursorArchitectAgentInput) error {
			if input.ArchitectContract.TargetRoot != "." {
				t.Fatalf("unexpected target root: %#v", input.ArchitectContract)
			}
			if input.Packet.Mode != "implementation_only" {
				t.Fatalf("cursor packet mode = %q", input.Packet.Mode)
			}
			if len(input.Packet.EditSurface) == 0 || input.Packet.EditSurface[0] != "main.go" {
				t.Fatalf("cursor packet edit surface = %#v", input.Packet.EditSurface)
			}
			if !stringListContains(input.Packet.Forbidden, "do not claim objective completion; Omnidex will run proof commands and decide completion") {
				t.Fatalf("cursor packet missing completion authority guardrail: %#v", input.Packet.Forbidden)
			}
			return os.WriteFile(filepath.Join(input.Workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
		},
	}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	contract := ImplementationArchitectContract{
		Role:       "implementation_architect",
		TargetRoot: ".",
		EditSurface: []string{
			"main.go",
		},
		WorkQueue: []ArchitectWorkItem{{
			ID:          "write_main",
			Operation:   "create",
			CWD:         ".",
			Path:        "main.go",
			Description: "Create the CLI entrypoint",
		}},
		CurrentItem: &ArchitectWorkItem{
			ID:          "write_main",
			Operation:   "create",
			CWD:         ".",
			Path:        "main.go",
			Description: "Create the CLI entrypoint",
		},
	}
	handled, err := runCursorArchitectAgentLane(
		context.Background(),
		2,
		"Create a Go CLI",
		"Implementation architect target root: . Create or modify the actual project files.",
		contract,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CursorArchitectAgent:    cursor,
		},
		WorksiteSurvey{},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected cursor architect lane to handle the task")
	}
	if len(cursor.inputs) != 1 {
		t.Fatalf("cursor agent calls = %d, want 1", len(cursor.inputs))
	}
	if !structuredEventsContain(events, "cursor_sdk_architect_agent_started") || !structuredEventsContain(events, "cursor_sdk_architect_agent_completed") || !structuredEventsContain(events, "cursor_sdk_architect_validation_passed") {
		t.Fatalf("missing cursor architect events: %#v", events)
	}
	if !hasImplementationArchitectProgress(result.Observations) {
		t.Fatalf("cursor architect result did not record architect progress: %#v", result.Observations)
	}
	if result.Observations[0].EvidenceKind != "implementation" || result.Observations[0].GeneratedBy != "cursor_sdk" {
		t.Fatalf("cursor result should be implementation evidence only: %#v", result.Observations[0])
	}
}

func TestBuildCursorArchitectPromptUsesMissionPacket(t *testing.T) {
	input := CursorArchitectAgentInput{
		UserPrompt: "build a notes app",
		Packet: CursorImplementationPacket{
			Task:        "Implement CRUD notes behavior",
			Mode:        "implementation_only",
			Workspace:   "/tmp/project",
			TargetRoot:  ".",
			EditSurface: []string{"src/App.jsx"},
			Objectives:  []string{"create note", "delete note"},
			ProofContract: CursorPacketProofContract{
				Commands:           []string{"npm run build"},
				EvidencePredicates: []string{"command_passed:npm run build"},
			},
			Forbidden: []string{"do not add react-router-dom"},
		},
	}
	prompt := buildCursorArchitectPrompt(input)
	for _, want := range []string{
		`"cursor_packet"`,
		`"mode": "implementation_only"`,
		`"edit_surface"`,
		`"proof_contract"`,
		"implementation evidence only",
		"Omnidex decides completion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("cursor prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"architect_contract"`) {
		t.Fatalf("cursor prompt should use mission packet instead of raw architect contract:\n%s", prompt)
	}
}

func TestBuildCodexArchitectPromptUsesMissionPacketAndNoCompletionAuthority(t *testing.T) {
	input := CursorArchitectAgentInput{
		UserPrompt: "build a notes app",
		Packet: CursorImplementationPacket{
			Task:        "Implement CRUD notes behavior",
			Mode:        "implementation_only",
			Workspace:   "/tmp/project",
			TargetRoot:  ".",
			EditSurface: []string{"src/App.jsx"},
			Objectives:  []string{"create note", "delete note"},
			ProofContract: CursorPacketProofContract{
				Commands:           []string{"npm run build"},
				EvidencePredicates: []string{"command_passed:npm run build"},
			},
			Forbidden: []string{"do not add react-router-dom"},
		},
	}
	prompt := buildCodexArchitectPrompt(input)
	for _, want := range []string{
		`"codex_packet"`,
		`"mode": "implementation_only"`,
		`"edit_surface"`,
		`"proof_contract"`,
		"implementation evidence only",
		"Omnidex will run proof commands",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("codex prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"architect_contract"`) {
		t.Fatalf("codex prompt should use mission packet instead of raw architect contract:\n%s", prompt)
	}
}

func TestSelectedExternalArchitectAgentPrefersCodexWhenConfigured(t *testing.T) {
	cursor := &fakeCursorArchitectAgent{}
	codex := &fakeCursorArchitectAgent{}
	agent, name := selectedExternalArchitectAgent(structuredCommandDecisionRunConfig{
		CursorArchitectAgent: cursor,
		CodexArchitectAgent:  codex,
	})
	if agent != codex || name != "codex_sdk" {
		t.Fatalf("selected agent = %#v %q, want codex", agent, name)
	}
}

func TestExternalArchitectAgentsRequireExplicitEnvSelection(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "cursor-key")
	t.Setenv("CODEX_API_KEY", "codex-key")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OMNI_ENABLE_CURSOR_ARCHITECT", "true")
	t.Setenv("OMNI_ENABLE_CODEX_ARCHITECT", "true")
	t.Setenv("OMNI_DISABLE_CURSOR_ARCHITECT", "false")
	t.Setenv("OMNI_DISABLE_CODEX_ARCHITECT", "false")
	t.Setenv("OMNI_ARCHITECT_AGENT", "")
	if NewCursorSDKArchitectAgentFromEnv() != nil || NewCodexSDKArchitectAgentFromEnv() != nil {
		t.Fatal("external architect agents should stay disabled unless OMNI_ARCHITECT_AGENT selects one")
	}
	t.Setenv("OMNI_ARCHITECT_AGENT", "cursor")
	if NewCursorSDKArchitectAgentFromEnv() == nil {
		t.Fatal("cursor architect should be configured when selected and enabled")
	}
	if NewCodexSDKArchitectAgentFromEnv() != nil {
		t.Fatal("codex architect should not be configured when cursor is selected")
	}
	t.Setenv("OMNI_ARCHITECT_AGENT", "codex")
	if NewCodexSDKArchitectAgentFromEnv() == nil {
		t.Fatal("codex architect should be configured when selected and enabled")
	}
	if NewCursorSDKArchitectAgentFromEnv() != nil {
		t.Fatal("cursor architect should not be configured when codex is selected")
	}
}

func TestExternalArchitectAgentStreamsNormalizedEvents(t *testing.T) {
	agent := &fakeStreamingArchitectAgent{events: []AgentEvent{
		{Type: "started", Message: "started"},
		{Type: "command", Message: "running build", Command: "npm run build"},
		{Type: "file_change", Message: "changed files", Files: []string{"src/App.jsx"}},
		{Type: "completed", Message: "finished"},
	}}
	events := []StructuredCommandEvent{}
	result, err := runExternalArchitectAgentTask(
		context.Background(),
		agent,
		"codex_sdk",
		CursorArchitectAgentInput{
			Workspace: t.TempDir(),
			Packet: CursorImplementationPacket{
				Mode:        "implementation_only",
				EditSurface: []string{"src/App.jsx"},
			},
		},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "finished" {
		t.Fatalf("summary = %q", result.Summary)
	}
	for _, want := range []string{"external_agent_started", "external_agent_command", "external_agent_file_change", "external_agent_completed"} {
		if !structuredEventsContain(events, want) {
			t.Fatalf("missing %s in %#v", want, events)
		}
	}
	commandEvent := structuredEventOfTypeForTest(events, "external_agent_command")
	if commandEvent == nil || commandEvent.Details["command"] != "npm run build" {
		t.Fatalf("command event missing command detail: %#v", commandEvent)
	}
}

func TestHumanCorrectionCancelsAndRestartsExternalAgentWithRevisedPacket(t *testing.T) {
	active := &fakeExternalAgentSession{}
	provider := &fakeStreamingArchitectAgent{events: []AgentEvent{{Type: "started", Message: "restarted"}}}
	input := CursorArchitectAgentInput{
		Workspace: t.TempDir(),
		Packet: CursorImplementationPacket{
			Mode:            "implementation_only",
			EditSurface:     []string{"src/App.jsx", "src/App.css"},
			Forbidden:       []string{"do not create a sibling project"},
			PreparedContext: []string{"repo summary"},
		},
	}
	events, revised, err := restartExternalAgentSessionWithCorrection(context.Background(), active, provider, "codex_sdk", input, HumanCorrection{
		Message:               "Do not add routing. Keep it single-page.",
		Authority:             "user",
		ForbiddenDependencies: []string{"react-router-dom"},
		AllowedFiles:          []string{"src/App.jsx"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.cancelCount != 1 || active.cleanupCount != 1 {
		t.Fatalf("active session cancel/cleanup = %d/%d, want 1/1", active.cancelCount, active.cleanupCount)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("provider inputs = %d, want 1", len(provider.inputs))
	}
	if got := revised.Packet.EditSurface; len(got) != 1 || got[0] != "src/App.jsx" {
		t.Fatalf("edit surface = %#v, want corrected allowed files", got)
	}
	if !testStringSliceContains(revised.Packet.Forbidden, "do not add dependency: react-router-dom") {
		t.Fatalf("forbidden missing dependency correction: %#v", revised.Packet.Forbidden)
	}
	if !testStringSliceContainsSubstring(revised.Packet.PreparedContext, "human_correction[user]: Do not add routing. Keep it single-page.") {
		t.Fatalf("prepared context missing human correction: %#v", revised.Packet.PreparedContext)
	}
	got := resultFromExternalAgentEvents(events)
	if got.Summary != "restarted" {
		t.Fatalf("summary = %q, want restarted", got.Summary)
	}
}

func testStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func testStringSliceContainsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func TestBuildCodeContentSpecialistRequestIncludesAuthoritativeFileContract(t *testing.T) {
	contract := ImplementationArchitectContract{
		AcceptanceCriteria: []string{"channel rack", "mixer controls", "visual timeline"},
	}
	cases := []struct {
		name string
		item ArchitectWorkItem
		want []string
	}{
		{
			name: "package_json",
			item: ArchitectWorkItem{Operation: "update", Path: "package.json"},
			want: []string{`"role":"npm_package_manifest"`, `"language":"json"`, "comments", "javascript module source"},
		},
		{
			name: "smoke_test",
			item: ArchitectWorkItem{Operation: "update", Path: "scripts/smoke-test.mjs"},
			want: []string{`"role":"deterministic_acceptance_probe"`, `"language":"node_javascript_module"`, "readFileSync", "process.exit"},
		},
		{
			name: "app_component",
			item: ArchitectWorkItem{Operation: "update", Path: "src/App.js"},
			want: []string{`"role":"react_application_component"`, `"language":"javascript_or_jsx_module"`, "channel rack", "mixer", "timeline"},
		},
		{
			name: "stylesheet",
			item: ArchitectWorkItem{Operation: "update", Path: "src/App.css"},
			want: []string{`"role":"css_stylesheet"`, `"language":"css"`, ".channel-rack", "react component"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := buildCodeContentSpecialistRequest(CodeContentSpecialistInput{
				UserPrompt:        "build a music studio",
				ArchitectContract: contract,
				WorkItem:          tc.item,
			})
			text := structuredRequestMessagesText(req)
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Fatalf("request missing %q:\n%s", want, text)
				}
			}
			if !strings.Contains(text, "file_contract is authoritative") {
				t.Fatalf("request missing authoritative file contract rule:\n%s", text)
			}
		})
	}
}

func TestValidateShellProposalRequiresArchitectTargetRoot(t *testing.T) {
	toolTask := "Recovery required. Implementation architect target root: react-music-production. All source edits, package scripts, and verification commands for this app must run inside react-music-production or use paths under react-music-production/."
	if err := validateShellProposalAgainstToolTask(`cat > src/App.js <<'JS'
export default function App() { return null; }
JS`, toolTask); err == nil {
		t.Fatal("expected root-relative source edit to be rejected")
	}
	for _, command := range []string{
		`cd react-music-production && cat > src/App.js <<'JS'
export default function App() { return null; }
JS`,
		`cat > react-music-production/src/App.js <<'JS'
export default function App() { return null; }
JS`,
	} {
		if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
			t.Fatalf("expected architect-targeted command to be allowed: %v", err)
		}
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskRejectsPlaceholderMutation(t *testing.T) {
	for _, command := range []string{"touch Clock.js", "mkdir -p src && touch src/Clock.js"} {
		err := validateShellProposalAgainstToolTask(command, "Required next behavior: create or modify the actual project files now. Do not create placeholder-only files with touch or empty mkdir scaffolds.")
		if err == nil {
			t.Fatalf("expected placeholder mutation %q to be rejected", command)
		}
		if !strings.Contains(err.Error(), "placeholder-only") {
			t.Fatalf("unexpected error for %q: %v", command, err)
		}
	}
}

func TestValidateShellProposalAgainstSourceImplementationRejectsDependencyInstall(t *testing.T) {
	toolTask := "Active objective(s): setup_note_app,create_note_app_structure,implement_crud_operations,store_notes_in_memory. Required next behavior: create or modify the actual project files now with substantive source/build/test files."
	for _, command := range []string{"npm install", "npm install react-router-dom", "pnpm add react-router-dom", "cargo add chess"} {
		err := validateShellProposalAgainstToolTask(command, toolTask)
		if err == nil {
			t.Fatalf("expected dependency install %q to be rejected for source implementation task", command)
		}
		if !strings.Contains(err.Error(), "source file implementation") {
			t.Fatalf("unexpected error for %q: %v", command, err)
		}
	}
}

func TestValidateShellProposalAllowsDependencyInstallWhenToolTaskRequiresDependencies(t *testing.T) {
	toolTask := "Active objective(s): install_dependencies. Required next behavior: install dependencies for the selected React project."
	if err := validateShellProposalAgainstToolTask("npm install react react-dom", toolTask); err != nil {
		t.Fatalf("dependency install should be allowed for dependency objective: %v", err)
	}
}

func TestValidateShellProposalPolicesDependencyInstallRationale(t *testing.T) {
	toolTask := "Active objective(s): install_dependencies. Required next behavior: install dependencies for the selected React project."
	err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		toolTask,
		"Installing react-router-dom will allow navigation between components, which is a common requirement in many React applications.",
	)
	if err == nil {
		t.Fatal("expected weak common-requirement rationale to be rejected")
	}
	if !strings.Contains(err.Error(), "without tool_task or evidence-backed rationale") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		"Install dependency react-router-dom for the requested routed React app.",
		"User requested routed navigation.",
	); err != nil {
		t.Fatalf("explicit tool task package should be allowed: %v", err)
	}
	if err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		toolTask,
		"Observed build error: Cannot find module react-router-dom imported by src/App.js.",
	); err != nil {
		t.Fatalf("evidence-backed rationale should be allowed: %v", err)
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskRejectsDocumentationDownload(t *testing.T) {
	err := validateShellProposalAgainstToolTask(
		"curl -s https://ziglang.org/documentation/master/ > zig_doc.html",
		"Required next behavior: create or modify the actual project files now with substantive source/build/test files.",
	)
	if err == nil {
		t.Fatal("expected documentation download to be rejected for source-write recovery")
	}
	if !strings.Contains(err.Error(), "documentation download") {
		t.Fatalf("unexpected error: %v", err)
	}
}
