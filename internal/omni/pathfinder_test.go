package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathfinderMissingFileAssumptionSelectsInspectSourceTree(t *testing.T) {
	problem := ProblemCase{
		Problem:     "file path was invalid; deterministic missing-file recovery required",
		CurrentGoal: "Build a React notes app",
		ObjectiveLedger: []StructuredObjective{{
			ID:     "implement_notes_crud",
			Status: "pending",
		}},
		RecentObservations: []StructuredCommandObservation{{
			Step:     1,
			Command:  "cat index.html",
			ExitCode: 1,
			Stderr:   "cat: index.html: No such file or directory",
		}},
		WorksiteSurvey: WorksiteSurvey{Evidence: []string{"package.json exists", "src/ exists"}},
	}

	packet := Pathfinder{}.Solve(problem)
	if err := ValidateBreakthroughPacket(problem, packet); err != nil {
		t.Fatal(err)
	}
	if packet.SelectedStrategy.ID != "inspect_workspace_shape" {
		t.Fatalf("selected strategy = %q, want inspect_workspace_shape", packet.SelectedStrategy.ID)
	}
	if packet.NextAction.Kind != string(PathfinderActionInspect) || !strings.Contains(packet.NextAction.Command, "package.json") || !strings.Contains(packet.NextAction.Command, "./src/*") {
		t.Fatalf("unexpected next action: %#v", packet.NextAction)
	}
	for _, forbidden := range []string{"./node_modules", "./.git", "./dist", "./build", "./coverage"} {
		if !strings.Contains(packet.NextAction.Command, forbidden) {
			t.Fatalf("inspect command should prune %s: %s", forbidden, packet.NextAction.Command)
		}
	}
}

func TestPathfinderInspectCommandPrunesDependencyDirectories(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"package.json", "src/App.jsx", "node_modules/vite/package.json", "dist/package.json"} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	packet := Pathfinder{}.Solve(ProblemCase{
		Problem:            "missing file assumption",
		CurrentGoal:        "inspect project",
		RecentObservations: []StructuredCommandObservation{{Command: "cat index.html", ExitCode: 1, Stderr: "No such file"}},
		WorksiteSurvey:     WorksiteSurvey{Evidence: []string{"package.json exists"}},
	})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, packet.NextAction.Command, root, false, "", stdout, stderr, nil, &result); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if strings.Contains(output, "node_modules") || strings.Contains(output, "dist/package.json") {
		t.Fatalf("inspect output included dependency/build files:\n%s", output)
	}
	if !strings.Contains(output, "./package.json") || !strings.Contains(output, "./src/App.jsx") {
		t.Fatalf("inspect output missing project files:\n%s", output)
	}
}

func TestPathfinderFalseDoneTightensCompletionProofWithoutCompleting(t *testing.T) {
	problem := ProblemCase{
		Problem:        "done=true rejected before completion validation",
		CurrentGoal:    "Build a notes app",
		FalseDoneCount: 1,
		ObjectiveLedger: []StructuredObjective{{
			ID:     "verify_build",
			Status: "pending",
		}},
		RecentObservations: []StructuredCommandObservation{{
			Step:     2,
			ExitCode: 1,
			Stderr:   "progression_gate: done=true rejected before completion validation",
		}},
	}

	packet := Pathfinder{}.Solve(problem)
	if err := ValidateBreakthroughPacket(problem, packet); err != nil {
		t.Fatal(err)
	}
	if packet.NextAction.Kind == "done" || packet.NextAction.Kind == "complete" {
		t.Fatalf("pathfinder cannot complete objectives: %#v", packet.NextAction)
	}
	if !strings.Contains(strings.ToLower(packet.RealBlocker), "completion") {
		t.Fatalf("real blocker should identify completion proof issue: %q", packet.RealBlocker)
	}
}

func TestPathfinderRejectsPacketWithNoEvidence(t *testing.T) {
	problem := ProblemCase{Problem: "stuck", CurrentGoal: "build app"}
	packet := BreakthroughPacket{
		Diagnosis:   "stuck",
		RealBlocker: "unknown",
		CandidateStrategies: []CandidateStrategy{
			{ID: "a", ActionKind: string(PathfinderActionInspect)},
			{ID: "b", ActionKind: string(PathfinderActionPatch)},
			{ID: "c", ActionKind: string(PathfinderActionRunVerification)},
		},
		NextAction:       PathfinderNextAction{Kind: string(PathfinderActionInspect), Command: "find . -maxdepth 1 -type f"},
		ProofNeededAfter: []string{"runtime observation"},
	}
	if err := ValidateBreakthroughPacket(problem, packet); err == nil {
		t.Fatal("expected no-evidence packet to be rejected")
	}
}

func TestPathfinderRejectsObjectiveCompletionAction(t *testing.T) {
	problem := ProblemCase{
		Problem:     "false done loop",
		CurrentGoal: "build app",
		ObjectiveLedger: []StructuredObjective{{
			ID:     "build_app",
			Status: "pending",
		}},
	}
	packet := Pathfinder{}.Solve(problem)
	packet.NextAction = PathfinderNextAction{Kind: "complete", Rationale: "model says done"}
	if err := ValidateBreakthroughPacket(problem, packet); err == nil {
		t.Fatal("expected completion action to be rejected")
	}
}

func TestPathfinderSelectedStrategyDiffersFromRecentlyExhaustedCommand(t *testing.T) {
	problem := ProblemCase{
		Problem:          "same command/output repeated without satisfying pending objectives",
		CurrentGoal:      "build app",
		RejectedCommands: []string{"npm run build"},
		ObjectiveLedger:  []StructuredObjective{{ID: "verify_build", Status: "pending"}},
	}
	packet := Pathfinder{}.Solve(problem)
	packet.NextAction = PathfinderNextAction{Kind: string(PathfinderActionRunVerification), Command: "npm run build"}
	if err := ValidateBreakthroughPacket(problem, packet); err == nil {
		t.Fatal("expected repeated exhausted command to be rejected")
	}
}

func TestPathfinderArtifactValidationFailureSelectsSourceRepairStrategy(t *testing.T) {
	problem := ProblemCase{
		Problem:     "artifact validation failed",
		CurrentGoal: "build app",
		ObjectiveLedger: []StructuredObjective{{
			ID:     "implement_ui",
			Status: "pending",
		}},
		RecentObservations: []StructuredCommandObservation{{
			Step:     3,
			ExitCode: 1,
			Stderr:   "artifact_validation_failed: src/App.jsx is empty",
		}},
	}
	packet := Pathfinder{}.Solve(problem)
	if err := ValidateBreakthroughPacket(problem, packet); err != nil {
		t.Fatal(err)
	}
	if packet.SelectedStrategy.ID != "repair_artifact_source" {
		t.Fatalf("selected strategy = %q, want repair_artifact_source", packet.SelectedStrategy.ID)
	}
	if packet.NextAction.Kind != string(PathfinderActionPatch) {
		t.Fatalf("next action kind = %q, want patch", packet.NextAction.Kind)
	}
}

func TestPathfinderActionDispatchesThroughRuntimeAndReturnsToLoopEvidence(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"scripts":{"build":"vite build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){return null}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := []StructuredCommandEvent{}
	result := &CommandDecisionResult{
		ObjectiveLedger: []StructuredObjective{{ID: "implement_app", Status: "pending"}},
		Observations: []StructuredCommandObservation{{
			Step:     1,
			Command:  "npm install",
			ExitCode: 1,
			Stdout:   "added 19 packages\n",
			Stderr:   "npm warning old lockfile\n",
		}, {
			Step:     2,
			Command:  "cat index.html",
			ExitCode: 1,
			Stderr:   "cat: index.html: No such file or directory",
		}},
	}
	decision := ProgressionDecision{
		Action:    ProgressForceRecovery,
		Reason:    "file path was invalid; deterministic missing-file recovery required",
		LoopState: structuredLoopStateFromState(result.ObjectiveLedger, result.Observations),
	}

	handled, err := runPathfinderForProgression(
		context.Background(),
		2,
		"Build a React app",
		decision,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
		WorksiteSurvey{Evidence: []string{"package.json exists", "src/ exists"}},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(event StructuredCommandEvent) { events = append(events, event) },
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected pathfinder inspect action to be handled")
	}
	for _, want := range []string{"pathfinder_started", "pathfinder_packet_validated", "pathfinder_action_dispatched", "pathfinder_action_result"} {
		if !structuredEventsContain(events, want) {
			t.Fatalf("missing %s in events %#v", want, events)
		}
	}
	if result.ExitCode != 0 || !strings.Contains(result.Command, "find . -maxdepth") {
		t.Fatalf("pathfinder command did not execute successfully: command=%q exit=%d", result.Command, result.ExitCode)
	}
	latest := result.Observations[len(result.Observations)-1]
	if !strings.Contains(latest.Command, "find . -maxdepth") {
		t.Fatalf("latest observation is not pathfinder find command: %#v", latest)
	}
	if strings.Contains(latest.Stdout, "added 19 packages") || strings.Contains(latest.Stderr, "old lockfile") {
		t.Fatalf("pathfinder action result mixed prior command output: %#v", latest)
	}
}
