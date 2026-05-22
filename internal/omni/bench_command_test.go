package omni

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOmniBenchListCommand(t *testing.T) {
	root := repoRootFromOmniTest(t)
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	if err := app.Run([]string{"bench", "list", "--root", filepath.Join(root, "benchmarks")}); err != nil {
		t.Fatalf("bench list failed: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "npm-stimulus-tailwind-calculator") {
		t.Fatalf("bench list output = %q", out.String())
	}
}

func TestOmniBenchListFindsInstalledBenchmarkRoot(t *testing.T) {
	installRoot := t.TempDir()
	benchmarkDir := filepath.Join(installRoot, "benchmarks", "sample")
	if err := os.MkdirAll(benchmarkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "id": "installed-benchmark",
  "description": "installed benchmark discovery",
  "prompt": "do a tiny task",
  "success_criteria": ["succeeds"]
}`
	if err := os.WriteFile(filepath.Join(benchmarkDir, "benchmark.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNIDEX_DIR", installRoot)
	cwd := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	if err := app.Run([]string{"bench", "list"}); err != nil {
		t.Fatalf("bench list failed: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "installed-benchmark") {
		t.Fatalf("bench list output = %q", out.String())
	}
}

func TestOmniBenchReportCommandPrintsSessionMetrics(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	sessionRoot := filepath.Join(tmp, "sessions")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(sessionRoot)
	session, _, err := store.LoadOrCreate(workspace)
	if err != nil {
		t.Fatal(err)
	}
	session.Turns = append(session.Turns, Turn{
		ID: "turn_000001",
		Events: []Event{
			{Type: "structured_llm_request_started", CreatedAt: "2026-05-20T12:00:00Z"},
			{Type: "structured_command_finished", CreatedAt: "2026-05-20T12:00:01Z"},
			{Type: "structured_command_rejected", CreatedAt: "2026-05-20T12:00:02Z"},
		},
	})
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	err = app.Run([]string{"bench", "report", "--workspace", workspace, "--session-root", sessionRoot})
	if err != nil {
		t.Fatalf("bench report failed: %v\nstderr=%s", err, errOut.String())
	}
	for _, want := range []string{"model_calls=1", "commands=1", "rejected_commands=1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("bench report missing %q: %s", want, out.String())
		}
	}
}

func TestOmniBenchRunDryRunCommand(t *testing.T) {
	root := t.TempDir()
	benchDir := filepath.Join(root, "sample")
	if err := os.MkdirAll(benchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "id": "dry-run-benchmark",
  "description": "dry run benchmark",
  "workspace": "tmp",
  "prompt": "prepare only",
  "success_criteria": ["dry run"]
}`
	if err := os.WriteFile(filepath.Join(benchDir, "benchmark.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	err := app.Run([]string{"bench", "run", "--root", root, "--run-root", t.TempDir(), "--session-root", filepath.Join(t.TempDir(), "sessions"), "--dry-run", "dry-run-benchmark"})
	if err != nil {
		t.Fatalf("bench run dry-run failed: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "success=true") {
		t.Fatalf("bench run output = %q", out.String())
	}
}

func TestOmniBenchSuiteAppGauntletDryRunCommand(t *testing.T) {
	root := t.TempDir()
	benchDir := filepath.Join(root, "sample")
	if err := os.MkdirAll(benchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "id": "sample-react-app",
  "description": "sample app benchmark",
  "workspace": "tmp",
  "prompt": "Create a small React app",
  "success_criteria": ["dry run"]
}`
	if err := os.WriteFile(filepath.Join(benchDir, "benchmark.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	err := app.Run([]string{"bench", "suite", "app-gauntlet", "--root", root, "--run-root", t.TempDir(), "--session-root", filepath.Join(t.TempDir(), "sessions"), "--dry-run"})
	if err != nil {
		t.Fatalf("bench suite dry-run failed: %v\nstderr=%s", err, errOut.String())
	}
	for _, want := range []string{"suite=app-gauntlet", "success=true", "benchmarks=1", "sample-react-app"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("bench suite output missing %q: %q", want, out.String())
		}
	}
}
