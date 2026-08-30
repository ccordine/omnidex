package workspace_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestPlanMutationDerivesOneDeterministicMixedWorkspaceDelta(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "replace.txt", "before\n", 0o600)
	writeWorkspaceFile(t, root, "remove.txt", "remove\n", 0o640)
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	desired := []workspace.DesiredFileState{
		{Path: "remove.txt", Source: workspaceSource(t, source, "remove.txt")},
		{Path: "created/nested.txt", Present: true, Content: []byte("created\n"), Mode: 0o644},
		{Path: "replace.txt", Source: workspaceSource(t, source, "replace.txt"), Present: true, Content: []byte("after\n"), Mode: 0o600},
	}
	plan, err := workspace.PlanMutation(t.Context(), source, "objective_"+strings.Repeat("a", 64), desired)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SourceStateID != source.ID || plan.ExpectedStateID == source.ID ||
		!strings.HasPrefix(plan.ID, "workspace_stage_") || len(plan.Files) != 3 {
		t.Fatalf("mutation plan=%+v", plan)
	}
	if got := []string{plan.Files[0].Path, plan.Files[1].Path, plan.Files[2].Path}; strings.Join(got, ",") != "created/nested.txt,remove.txt,replace.txt" {
		t.Fatalf("ordered transitions=%v", got)
	}
	reversed := append([]workspace.DesiredFileState(nil), desired...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	again, err := workspace.PlanMutation(t.Context(), source, plan.OwnerID, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != plan.ID || again.Patch != plan.Patch || again.ExpectedStateID != plan.ExpectedStateID {
		t.Fatalf("reordered desired state changed plan identity")
	}
	result, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: t.Context(), Workspace: root, Patch: plan.Patch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("applied files=%+v", result.Files)
	}
	after, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != plan.ExpectedStateID {
		t.Fatalf("post state=%s expected=%s", after.ID, plan.ExpectedStateID)
	}
	if err := plan.VerifyExpected(after); err != nil {
		t.Fatal(err)
	}
}

func TestPlanMutationFiltersExactStatesAndRejectsZeroDelta(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "same.txt", "same\n", 0o600)
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	exact := workspace.DesiredFileState{
		Path: "same.txt", Source: workspaceSource(t, source, "same.txt"),
		Present: true, Content: []byte("same\n"), Mode: 0o600,
	}
	absent := workspace.DesiredFileState{Path: "absent.txt"}
	if _, err := workspace.PlanMutation(t.Context(), source, "objective_"+strings.Repeat("b", 64), []workspace.DesiredFileState{exact, absent}); !errors.Is(err, workspace.ErrDesiredStateAlreadyExact) {
		t.Fatalf("zero-delta error=%v", err)
	}
	created := workspace.DesiredFileState{Path: "created.txt", Present: true, Content: []byte("new\n"), Mode: 0o644}
	plan, err := workspace.PlanMutation(t.Context(), source, "objective_"+strings.Repeat("b", 64), []workspace.DesiredFileState{exact, created, absent})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != "created.txt" {
		t.Fatalf("filtered plan files=%+v", plan.Files)
	}
}

func TestPlanMutationRejectsStaleAmbiguousAndProtectedAuthority(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "value.txt", "one\n", 0o600)
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	owner := "objective_" + strings.Repeat("c", 64)
	tests := []struct {
		name    string
		desired workspace.DesiredFileState
		want    string
	}{
		{name: "missing source", desired: workspace.DesiredFileState{Path: "value.txt", Present: true, Content: []byte("two\n"), Mode: 0o600}, want: "source authority"},
		{name: "false source", desired: workspace.DesiredFileState{Path: "missing.txt", Source: workspaceSource(t, source, "value.txt"), Present: true, Content: []byte("two\n"), Mode: 0o644}, want: "absent"},
		{name: "protected", desired: workspace.DesiredFileState{Path: ".omni/state", Present: true, Content: []byte("two\n"), Mode: 0o644}, want: "protected"},
		{name: "dependency", desired: workspace.DesiredFileState{Path: "node_modules/pkg/index.js", Present: true, Content: []byte("two\n"), Mode: 0o644}, want: "protected"},
		{name: "secret", desired: workspace.DesiredFileState{Path: ".env", Present: true, Content: []byte("two\n"), Mode: 0o644}, want: "protected"},
		{name: "create mode", desired: workspace.DesiredFileState{Path: "new.txt", Present: true, Content: []byte("two\n"), Mode: 0o600}, want: "mode 0644"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := workspace.PlanMutation(t.Context(), source, owner, []workspace.DesiredFileState{test.desired}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want=%q", err, test.want)
			}
		})
	}
	writeWorkspaceFile(t, root, "value.txt", "drifted\n", 0o600)
	if _, err := workspace.PlanMutation(t.Context(), source, owner, []workspace.DesiredFileState{{
		Path: "value.txt", Source: workspaceSource(t, source, "value.txt"),
		Present: true, Content: []byte("two\n"), Mode: 0o600,
	}}); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("drift error=%v", err)
	}
}

func TestPlanMutationEnforcesThirtyTwoFileAndContentBounds(t *testing.T) {
	root := t.TempDir()
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	desired := make([]workspace.DesiredFileState, 33)
	for index := range desired {
		desired[index] = workspace.DesiredFileState{
			Path: fmt.Sprintf("file-%02d.txt", index), Present: true,
			Content: []byte("value\n"), Mode: 0o644,
		}
	}
	owner := "objective_" + strings.Repeat("d", 64)
	if _, err := workspace.PlanMutation(t.Context(), source, owner, desired); err == nil || !strings.Contains(err.Error(), "1-32") {
		t.Fatalf("33-file error=%v", err)
	}
	desired = desired[:32]
	if _, err := workspace.PlanMutation(t.Context(), source, owner, desired); err != nil {
		t.Fatal(err)
	}
	desired[0].Content = append(make([]byte, workspace.MaxMutationFileBytes), '\n')
	if _, err := workspace.PlanMutation(t.Context(), source, owner, desired); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("content-bound error=%v", err)
	}
}

func workspaceSource(t *testing.T, snapshot workspace.Snapshot, path string) *workspace.ExactSourceFile {
	t.Helper()
	for _, entry := range snapshot.Entries {
		if entry.Path == path {
			return &workspace.ExactSourceFile{
				EntryID: entry.ID, SHA256: entry.SHA256, Size: entry.Size, Mode: entry.Mode,
			}
		}
	}
	t.Fatalf("workspace snapshot has no entry %q", path)
	return nil
}

func TestBuildFullFileUnifiedPatchRejectsInvalidTransitions(t *testing.T) {
	if _, err := workspace.BuildFullFileUnifiedPatch("value.txt", false, nil, false, nil); err == nil {
		t.Fatal("absent-to-absent patch succeeded")
	}
	if _, err := workspace.BuildFullFileUnifiedPatch(filepath.Join("..", "value.txt"), false, nil, true, []byte("x\n")); err == nil {
		t.Fatal("escaping patch path succeeded")
	}
	if _, err := workspace.BuildFullFileUnifiedPatch("value.txt", false, nil, true, []byte("missing-newline")); err == nil {
		t.Fatal("unterminated desired text succeeded")
	}
}
