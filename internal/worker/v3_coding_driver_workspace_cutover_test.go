package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestDirectCodingDriverHasOneJournaledWorkspaceMutationPath(t *testing.T) {
	workflow := mustReadDirectCodingCutoverSource(t, "v3_coding_workflow.go")
	files := mustReadDirectCodingCutoverSource(t, "v3_coding_driver_files.go")
	execute := mustReadDirectCodingCutoverSource(t, "v3_coding_driver_workspace_execute.go")
	verification := mustReadDirectCodingCutoverSource(t, "v3_coding_driver_verification.go")

	for _, required := range []struct {
		source string
		value  string
	}{
		{workflow, "PrepareAssembly("},
		{workflow, "ApplyAndVerify("},
		{files, "workspacefacts.PlanMutation("},
		{files, "workspacefacts.StageMutation("},
		{files, "workspaceMutationCommandForStage("},
		{execute, ".ExecuteWorkspaceMutation("},
		{execute, "prepared.stage.ApplyVerified("},
		{execute, "collectDirectCodingWorkspaceVerification("},
	} {
		if !strings.Contains(required.source, required.value) {
			t.Fatalf("direct-coding cutover lacks %q", required.value)
		}
	}
	for _, forbidden := range []struct {
		source string
		value  string
	}{
		{workflow, "MaterializeTask("},
		{workflow, "EnsureDirectory("},
		{files, "os.WriteFile("},
		{files, "executeCodeCommandAtRoot("},
		{verification, "executeCodeCommandAtRoot("},
	} {
		if strings.Contains(forbidden.source, forbidden.value) {
			t.Fatalf("direct-coding cutover retains %q", forbidden.value)
		}
	}
}

func TestDirectCodingDriverOmitsFullTreeDockerCopyOut(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("v3_coding_driver*.go")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "v3_coding_workflow.go")
	forbidden := map[string]string{
		"docker cp":         "Docker copy",
		`"cp",`:             "copy subprocess",
		"archive/tar":       "whole-tree archive transport",
		"tar.NewReader(":    "whole-tree archive reader",
		"tar.NewWriter(":    "whole-tree archive writer",
		"filepath.Walk(":    "whole-tree traversal",
		"filepath.WalkDir(": "whole-tree traversal",
		"io.Copy(":          "byte-stream copy-out",
		"os.ReadFile(":      "host file byte read",
		"os.WriteFile(":     "host file byte write",
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source := mustReadDirectCodingCutoverSource(t, path)
		for token, label := range forbidden {
			if strings.Contains(source, token) {
				t.Fatalf("direct-coding source %s retains %s %q", path, label, token)
			}
		}
	}
	invocation := mustReadDirectCodingCutoverSource(
		t, "v3_coding_driver_workspace_sandbox_invocation.go",
	)
	for _, required := range []string{"--bind-fd", "--ro-bind", "repositoryWorkspaceProjection"} {
		if !strings.Contains(invocation, required) {
			t.Fatalf("direct-coding sandbox lacks projection mount authority %q", required)
		}
	}
}

func TestPreparedWorkspaceMutationAccountingIsReplayIdempotent(t *testing.T) {
	session := directCodingSession{completion: directCodingCompletionState{
		WrittenSource: make(map[string]string),
	}}
	prepared := &directCodingPreparedMutation{
		assembly: directCodingAssembly{Files: []directCodingFileTask{{
			Path: "main.go", Content: "package main\n",
		}}},
		command: queue.WorkspaceMutationCommand{Plan: workspacefacts.MutationPlan{
			ExpectedStateID: strings.Repeat("a", 64),
			Files: []workspacefacts.MutationFileTransition{{
				Path: "main.go",
				Expected: workspacefacts.MutationFileState{
					Present: true, SHA256: strings.Repeat("b", 64), Size: 13, Mode: 0o644,
				},
			}},
		}},
	}
	session.recordPreparedWorkspaceMutation(prepared)
	session.recordPreparedWorkspaceMutation(prepared)
	if session.completion.MutationCount != 1 || len(session.mutationJournal) != 1 ||
		session.completion.WrittenSource["main.go"] != "package main\n" {
		t.Fatalf(
			"replayed accounting count=%d journal=%+v source=%q",
			session.completion.MutationCount, session.mutationJournal,
			session.completion.WrittenSource["main.go"],
		)
	}
}

func mustReadDirectCodingCutoverSource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
