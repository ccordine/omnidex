package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestRepositorySemanticStageConvergesIntoGenericWorkspaceMutation(t *testing.T) {
	source, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	stage, err := stageWorkspaceMutationFromRepositoryChange(
		t.Context(), source, contract.ID, commands, prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	plan := stage.Plan()
	if plan.OwnerID != contract.ID || plan.GitSourceSnapshotID != source.ID ||
		plan.WorkspaceRoot != source.Root || len(plan.Files) != 1 {
		t.Fatalf("generic workspace plan=%+v", plan)
	}
	projection, err := newWorkspaceStagedProjection(t.Context(), stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.VerifyExact(t.Context()); err != nil {
		t.Fatal(err)
	}
	command := queue.WorkspaceMutationCommand{ProjectLocation: source.Root, Plan: plan}
	observed, err := observeWorkspaceMutation(t.Context(), command)
	if err != nil || observed != queue.WorkspaceMutationSource {
		t.Fatalf("source observation=%q error=%v", observed, err)
	}
	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	observed, err = observeWorkspaceMutation(t.Context(), command)
	if err != nil || observed != queue.WorkspaceMutationPost {
		t.Fatalf("post observation=%q error=%v", observed, err)
	}
	if err := os.WriteFile(filepath.Join(source.Root, "unexpected.txt"), []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err = observeWorkspaceMutation(t.Context(), command)
	if err != nil || observed != queue.WorkspaceMutationIndeterminate {
		t.Fatalf("drift observation=%q error=%v", observed, err)
	}
}

func TestWorkspaceMutationStageAuthorityBindsCurrentProjectAndRuntimeRoots(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := t.TempDir()
	runtimeProject := filepath.Join(runtimeRoot, "parcel-service")
	hostProject := filepath.Join(hostRoot, "parcel-service")
	for _, root := range []string{runtimeProject, hostProject} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	metadata, err := json.Marshal(map[string]string{"client_cwd": hostProject})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{workspaceRoot: runtimeRoot, workspaceHostRoot: hostRoot}
	job := model.Job{Metadata: metadata}
	project := model.Project{ID: 41, Location: hostProject}
	plan := workspacefacts.MutationPlan{WorkspaceRoot: runtimeProject}

	if err := requireWorkspaceMutationStageAuthority(service, job, project.ID, project, plan); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMutationStageAuthorityRejectsEitherRootDriftWithoutFallback(t *testing.T) {
	runtimeRoot := t.TempDir()
	hostRoot := t.TempDir()
	runtimeProject := filepath.Join(runtimeRoot, "climate-console")
	hostProject := filepath.Join(hostRoot, "climate-console")
	otherRuntimeProject := filepath.Join(runtimeRoot, "other-console")
	otherHostProject := filepath.Join(hostRoot, "other-console")
	for _, root := range []string{
		runtimeProject, hostProject, otherRuntimeProject, otherHostProject,
	} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	metadata, err := json.Marshal(map[string]string{"client_cwd": hostProject})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{workspaceRoot: runtimeRoot, workspaceHostRoot: hostRoot}
	job := model.Job{Metadata: metadata}
	project := model.Project{ID: 73, Location: hostProject}
	plan := workspacefacts.MutationPlan{WorkspaceRoot: runtimeProject}

	tests := []struct {
		name      string
		projectID int64
		project   model.Project
		plan      workspacefacts.MutationPlan
		want      string
	}{
		{
			name: "project identity", projectID: project.ID + 1,
			project: project, plan: plan, want: "project authority is incomplete",
		},
		{
			name: "stored project location", projectID: project.ID,
			project: model.Project{ID: project.ID, Location: otherHostProject},
			plan:    plan, want: "job location differs",
		},
		{
			name: "runtime plan root", projectID: project.ID,
			project: project,
			plan:    workspacefacts.MutationPlan{WorkspaceRoot: otherRuntimeProject},
			want:    "plan root differs",
		},
		{
			name: "missing stored project location", projectID: project.ID,
			project: model.Project{ID: project.ID}, plan: plan,
			want: "current project location is not canonical",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := requireWorkspaceMutationStageAuthority(
				service, job, test.projectID, test.project, test.plan,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("authority error=%v want substring %q", err, test.want)
			}
		})
	}
}
