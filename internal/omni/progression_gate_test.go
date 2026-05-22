package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProgressionGateForcesRecoveryForExhaustedCommand(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt: "finish calculator app",
		ObjectiveLedger: []StructuredObjective{
			{ID: "implement_calculator_ui", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 1, Stderr: "install failed"},
			{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
			{Step: 3, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=3"},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{
		"Recovery required.",
		"Active objective(s): implement_calculator_ui",
		"actual project files",
	} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
	for _, forbidden := range []string{"Blocked command(s):", "Forbidden command(s):"} {
		if strings.Contains(decision.RecoveryToolTask, forbidden) {
			t.Fatalf("recovery task should not contain %q: %s", forbidden, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateFailsCleanlyWhenRecoveryIsExhausted(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	gate := ProgressionGate{MaxRecoveryAttempts: 1}
	decision := gate.ReviewStep(ProgressionInput{
		ObjectiveLedger: []StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 1, Stderr: "install failed"},
			{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
			{Step: 3, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=3"},
			{Step: 4, ExitCode: 1, Stderr: "progression_gate: forced recovery required; repeated command failed to advance; deterministic recovery required"},
		},
	})

	if decision.Action != ProgressFailWithEvidence {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressFailWithEvidence)
	}
	if !strings.Contains(decision.Reason, "recovery exhausted") {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestProgressionGateAllowsDifferentFailureFingerprint(t *testing.T) {
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		ObjectiveLedger: []StructuredObjective{{ID: "verify_ui_and_logic", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "go test ./internal/omni -run TestFoo", ExitCode: 1, Stderr: "expected 1 got 0"},
			{Step: 2, Command: "go test ./internal/omni -run TestFoo", ExitCode: 1, Stderr: "expected 2 got 1"},
		},
	})

	if decision.Action != ProgressAllow {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressAllow)
	}
}

func TestProgressionGateForcesRecoveryAfterRepeatedSameFailureFingerprint(t *testing.T) {
	command := "npx tailwindcss init -p"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:          "Build a React clock app with Tailwind",
		ObjectiveLedger: []StructuredObjective{{ID: "install_and_integrate_tailwindcss", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 1, Stderr: "npm error could not determine executable to run"},
			{Step: 2, Command: command, ExitCode: 1, Stderr: "npm error could not determine executable to run"},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{"same command produced the same result", command, "could not determine executable", "inspect package.json", "directly instead of repeating"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("no-progress recovery missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateForcesRecoveryAfterRepeatedNoopPackageInstall(t *testing.T) {
	command := "npm install -D tailwindcss postcss autoprefixer"
	output := "up to date, audited 19 packages in 553ms\n\nfound 0 vulnerabilities"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:          "Build a React clock app with Tailwind",
		ObjectiveLedger: []StructuredObjective{{ID: "create_hello_world_component", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 0, Stdout: output},
			{Step: 2, Command: command, ExitCode: 0, Stdout: output},
			{Step: 3, Command: command, ExitCode: 0, Stdout: output},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	if !strings.Contains(decision.RecoveryToolTask, "do not retry the same command") {
		t.Fatalf("recovery task = %s", decision.RecoveryToolTask)
	}
}

func TestProgressionGateDoesNotForceRecoveryForEmptyProjectFilesMidLoop(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.test.js"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:     "finish this React notes app",
		WorkingDir: workspace,
		ObjectiveLedger: []StructuredObjective{{
			ID:       "create_noteslist_component",
			Status:   "pending",
			Source:   structuredObjectiveSourceUserExplicit,
			Required: true,
		}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "touch src/App.test.js", ExitCode: 0},
		},
	})

	if decision.Action == ProgressForceRecovery && strings.Contains(decision.Reason, "empty project files") {
		t.Fatalf("empty files should be a completion gate, not mid-loop progression recovery: %#v", decision)
	}
}

func TestProgressionGateContinuesAfterExistingGoReactScaffold(t *testing.T) {
	command := "mkdir -p backend/calculus-api && cd backend/calculus-api && go mod init calculus-api && cd ../.. && mkdir -p frontend/calculus-frontend && cd frontend/calculus-frontend && npx create-react-app ."
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt: "Build a complete calculus learning and solving app using Go for the backend and React JS for the frontend.",
		ObjectiveLedger: []StructuredObjective{
			{ID: "implement_backend_api", Status: "pending"},
			{ID: "implement_react_frontend", Status: "pending"},
			{ID: "verify_tests", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 0, Stdout: "go: creating new go.mod: module calculus-api\nSuccess! Created calculus-frontend"},
			{Step: 2, Command: command, ExitCode: 1, Stderr: "go: /tmp/demo/backend/calculus-api/go.mod already exists"},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	if decision.RejectedCommand != command {
		t.Fatalf("rejected command = %q, want scaffold command", decision.RejectedCommand)
	}
	for _, want := range []string{
		"project scaffold already exists",
		"setup/scaffold commands must not be rerun",
		"create or modify the actual backend and frontend project files now",
		"Go plus React",
		"go test ./...",
		"npm test",
	} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestExistingScaffoldRecoveryIncludesNestedTargetRoot(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"react-scripts build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "public", "index.html"), []byte(`<div id="root"></div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "src", "index.js"), []byte(`import './App';`), 0o644); err != nil {
		t.Fatal(err)
	}

	task := existingScaffoldRecoveryToolTask("build a React app", nil, StructuredCommandObservation{
		Command: "npx create-react-app react-music-production",
		Stdout:  "Success! Created react-music-production at /tmp/demo/react-music-production",
	}, workspace)
	if !strings.Contains(task, "Implementation architect target root: react-music-production") {
		t.Fatalf("recovery task missing target root: %s", task)
	}
}

func TestProgressionGateForcesDockerLifecycleAfterDockerfileOnlyProgress(t *testing.T) {
	command := "echo 'Creating Dockerfile...' && echo 'FROM nginx:alpine' > Dockerfile && echo 'Dockerfile created successfully.'"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt: "Dockerize this app, build the Docker image, and run it in a container.",
		ObjectiveLedger: []StructuredObjective{
			{ID: "create_dockerfile", Status: "satisfied"},
			{ID: "determine_docker_compatibility", Status: "pending"},
			{ID: "include_dependencies_in_docker_image", Status: "pending"},
			{ID: "build_docker_image", Status: "pending"},
			{ID: "run_application_in_docker_container", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 0, Stdout: "Creating Dockerfile...\nDockerfile created successfully."},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{"Dockerfile was created", "inspect the current Dockerfile", "docker build", "docker run", "curl", "docker inspect", "docker logs", "iterate over the Dockerfile", "Do not return done=true"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateUsesCompletedEvidenceForRepeatedSuccess(t *testing.T) {
	command := "ls -la /tmp/demo"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt: "connect calculator UI to logic",
		ObjectiveLedger: []StructuredObjective{
			{ID: "create_calculator_ui", Status: "pending"},
			{ID: "connect_ui_to_logic", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 2, Command: command, ExitCode: 0, Stdout: "package.json\nsrc\n"},
			{Step: 4, Command: "SKIPPED_REPEAT_SUCCESS: " + command, RejectedCommand: command, ExitCode: 0, Stdout: "already_completed"},
		},
	})

	if decision.Action != ProgressUseCompletedEvidence {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressUseCompletedEvidence)
	}
	if decision.PreviousResult == nil || !strings.Contains(decision.PreviousResult.Stdout, "package.json") {
		t.Fatalf("previous result missing stdout: %#v", decision.PreviousResult)
	}
	for _, want := range []string{"Use the previous command output", "package.json", "src", "Do not return done=true"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateBuildsMissingFileRecovery(t *testing.T) {
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:          "connect calculator UI to logic",
		ObjectiveLedger: []StructuredObjective{{ID: "create_calculator_ui", Status: "pending"}},
		Observations: []StructuredCommandObservation{{
			Step:     1,
			Command:  "cat /tmp/demo/index.html",
			ExitCode: 1,
			Stderr:   "cat: /tmp/demo/index.html: No such file or directory",
		}},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{"target path does not exist", "ls -la /tmp/demo", "find /tmp/demo -maxdepth 3 -type f", "Do not retry the invalid path"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("missing-file recovery missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateForcesWriteAfterRepeatedInspectionForMissingAppFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:     "Build a complete calculator app with HTML and JavaScript",
		WorkingDir: workspace,
		ObjectiveLedger: []StructuredObjective{
			{ID: "create_calculator_ui", Status: "pending"},
			{ID: "implement_calculator_logic", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "npm list --depth=0", ExitCode: 0, Stdout: "webpack\n"},
			{Step: 2, Command: "ls -la", ExitCode: 0, Stdout: "package.json\nsrc\n"},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{"inspected enough", "create or modify", "substantive source", "smallest hello-world project", "compiler build/test", "Already completed read-only command(s): npm list --depth=0; ls -la"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("write recovery missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
	if strings.Contains(decision.RecoveryToolTask, "Forbidden next command(s):") {
		t.Fatalf("write recovery should not label inspection evidence as forbidden: %s", decision.RecoveryToolTask)
	}
}

func TestProgressionGateRejectsPlaceholderOnlySuccessForAppBuild(t *testing.T) {
	decision := ProgressionGate{}.ReviewStep(ProgressionInput{
		Prompt:     "Build a Zig CLI calculator application.",
		WorkingDir: t.TempDir(),
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "mkdir -p src", ExitCode: 0},
			{Step: 2, Command: "touch src/main.zig", ExitCode: 0},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	if decision.RejectedCommand != "touch src/main.zig" {
		t.Fatalf("rejected command = %q", decision.RejectedCommand)
	}
	for _, want := range []string{"substantive source", "smallest hello-world project", "Do not create placeholder-only"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestStructuredLoopRecoveryUsesWriteRecoveryForPendingAppObjectives(t *testing.T) {
	task := structuredLoopRecoveryToolTask(
		"please continue setting up this project as a react js note app",
		[]StructuredObjective{
			{ID: "install_react_dependencies", Status: "satisfied", Evidence: "npm install exited 0"},
			{ID: "create_note_app_structure", Description: "Create note app component structure", Status: "pending"},
			{ID: "implement_crud_operations", Description: "Implement CRUD operations", Status: "pending"},
			{ID: "store_notes_in_memory", Description: "Store notes in memory", Status: "pending"},
		},
		[]StructuredCommandObservation{
			{Step: 1, Command: "npm install", ExitCode: 0, Stdout: "up to date"},
			{Step: 2, RejectedCommand: "echo 'Creating components and state management...'", ExitCode: 1, Stderr: "pure echo command is not command evidence"},
			{Step: 3, RejectedCommand: "npm install react-router-dom", ExitCode: 1, Stderr: "dependency scope drift"},
		},
	)

	for _, want := range []string{"create or modify the actual project files now", "substantive source", "Do not create placeholder-only", "Pending objective(s): create_note_app_structure,implement_crud_operations,store_notes_in_memory"} {
		if !strings.Contains(task, want) {
			t.Fatalf("recovery task missing %q: %s", want, task)
		}
	}
	if strings.Contains(task, "install_react_dependencies") {
		t.Fatalf("satisfied install objective should not remain active in recovery task: %s", task)
	}
}

func TestStructuredLoopRecoveryDoesNotTreatDockerApplicationObjectiveAsSourceWrite(t *testing.T) {
	task := structuredLoopRecoveryToolTask(
		"containerize this existing project",
		[]StructuredObjective{
			{ID: "create_dockerfile", Status: "satisfied", Evidence: "Dockerfile written"},
			{ID: "run_application_in_docker_container", Description: "Run application in Docker container", Status: "pending"},
		},
		[]StructuredCommandObservation{
			{Step: 1, Command: "docker build -t app .", ExitCode: 1, Stderr: "docker daemon unavailable"},
			{Step: 2, Command: "docker build -t app .", ExitCode: 1, Stderr: "docker daemon unavailable"},
		},
	)

	if strings.Contains(task, "substantive source") {
		t.Fatalf("docker application objective should not force source-write recovery: %s", task)
	}
	if !strings.Contains(task, "run_application_in_docker_container") {
		t.Fatalf("docker pending objective missing from recovery task: %s", task)
	}
}

func TestProgressionGateRejectsDocumentationDownloadAsAppMutation(t *testing.T) {
	workspace := t.TempDir()
	decision := ProgressionGate{}.ReviewStep(ProgressionInput{
		Prompt:     "Build a Zig CLI calculator application.",
		WorkingDir: workspace,
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "curl -s https://ziglang.org/documentation/master/ > zig_doc.html", ExitCode: 0},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	if !strings.Contains(decision.Reason, "substantive app source") {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
	if !strings.Contains(decision.RecoveryToolTask, "substantive source") {
		t.Fatalf("recovery task should require source writes: %s", decision.RecoveryToolTask)
	}
}

func TestProgressionGateForcesRecoveryAfterRepeatedPlannerNoopsForEmptyApp(t *testing.T) {
	decision := ProgressionGate{}.ReviewStep(ProgressionInput{
		Prompt:     "Build a Zig CLI calculator application.",
		WorkingDir: t.TempDir(),
		Observations: []StructuredCommandObservation{
			{Step: 1, RejectedResponse: `{"command":"","done":false,"answer":"workspace is empty"}`, ExitCode: 1, EvaluationFeedback: "workspace is empty and has no meaningful project files"},
			{Step: 2, RejectedResponse: `{"command":"","done":false,"answer":"initialize project"}`, ExitCode: 1, EvaluationFeedback: "initialize a new Zig project"},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	if !strings.Contains(decision.Reason, "planner repeatedly failed") {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}

func TestWorkspaceMissingAppFilesAcceptsZigProjectFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "build.zig"), []byte("const std = @import(\"std\");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "main.zig"), []byte("pub fn main() void {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if workspaceMissingAppFiles(workspace) {
		t.Fatal("complete Zig project files should satisfy app-file presence")
	}
}

func TestWorkspaceMissingAppFilesAcceptsRustCargoProjectFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "Cargo.toml"), []byte("[package]\nname=\"x\"\nversion=\"0.1.0\"\nedition=\"2024\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "main.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if workspaceMissingAppFiles(workspace) {
		t.Fatal("complete Rust Cargo project files should satisfy app-file presence")
	}
}

func TestWorkspaceMissingAppFilesAcceptsNestedReactProjectFiles(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"react-scripts build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "public", "index.html"), []byte(`<div id="root"></div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "src", "index.js"), []byte(`import './App';`), 0o644); err != nil {
		t.Fatal(err)
	}

	if workspaceMissingAppFiles(workspace) {
		t.Fatal("nested React project files should satisfy app-file presence for parent workspace")
	}
	if got := firstNestedAppRootWithFiles(workspace); got != "react-music-production" {
		t.Fatalf("nested app root = %q", got)
	}
}

func TestProgressionGateDoesNotForceWriteForCleanupObjectives(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "index.html"), []byte(`<script type="module" src="/src/main.jsx"></script>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "main.jsx"), []byte(`import App from './App.jsx';`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "Clock.js"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:     "Finish QA on this existing React clock app. Inspect empty placeholder files, remove them if unused, then verify the app.",
		WorkingDir: workspace,
		ObjectiveLedger: []StructuredObjective{
			{ID: "remove_empty_placeholder_files", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "verify_app_with_build", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "find . -name 'Clock.js'", ExitCode: 0, Stdout: "./src/Clock.js\n"},
			{Step: 2, Command: "ls -l src", ExitCode: 0, Stdout: "-rw-r--r-- 0 Clock.js\n"},
		},
	})

	if decision.Action != ProgressAllow {
		t.Fatalf("action = %s, want %s; decision=%#v", decision.Action, ProgressAllow, decision)
	}
}

func TestProgressionGateDoesNotForceWriteAfterMutation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:          "Build a complete calculator app with HTML and JavaScript",
		WorkingDir:      workspace,
		ObjectiveLedger: []StructuredObjective{{ID: "create_calculator_ui", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "ls -la", ExitCode: 0, Stdout: "package.json\nsrc\n"},
			{Step: 2, Command: "cat > index.html <<'HTML'\n<div></div>\nHTML", ExitCode: 0},
		},
	})

	if decision.Action != ProgressAllow {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressAllow)
	}
}
