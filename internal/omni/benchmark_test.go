package omni

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestLoadBenchmarkManifests(t *testing.T) {
	root := repoRootFromOmniTest(t)
	manifests, err := LoadBenchmarkManifests(filepath.Join(root, "benchmarks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatal("expected benchmark manifests")
	}
	found := false
	for _, manifest := range manifests {
		if manifest.ID == "npm-stimulus-tailwind-calculator" {
			found = true
			if manifest.Recipe != "frontend.stimulus-tailwind-recyclr" {
				t.Fatalf("recipe = %q", manifest.Recipe)
			}
		}
	}
	if !found {
		t.Fatalf("frontend benchmark not found: %#v", manifests)
	}
}

func TestBenchmarkReportFromSessionWarnsOnLoopExhaustion(t *testing.T) {
	report := BenchmarkReportFromSession(&Session{
		WorkspacePath: "/tmp/project",
		WorkspaceHash: "bench",
		Turns: []Turn{{
			Events: []Event{
				{Type: "structured_loop_exhausted"},
				{Type: "structured_command_rejected"},
			},
		}},
	})
	if report.LoopExhaustions != 1 || report.RejectedCommands != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected warning for loop exhaustion")
	}
}

func TestRunBenchmarkManifestExecutesInIsolatedWorkspace(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf '{}' > package.json","done":false,"answer":""}`,
		`{"command":"test -f package.json && cat package.json","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"created package.json"}`,
	}}
	result, err := RunBenchmarkManifest(
		context.Background(),
		BenchmarkManifest{
			ID:              "package-json-smoke",
			Description:     "create package json",
			Workspace:       "tmp",
			Prompt:          "Create package.json.",
			SuccessCriteria: []string{"package.json exists"},
		},
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		BenchmarkRunOptions{Root: t.TempDir(), SessionRoot: filepath.Join(t.TempDir(), "sessions")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("benchmark should succeed: %#v", result)
	}
	if !fileExists(filepath.Join(result.Workspace, "package.json")) {
		t.Fatalf("package.json missing in %s", result.Workspace)
	}
	if result.Report.Commands == 0 {
		t.Fatalf("report missing command count: %#v", result.Report)
	}
}
