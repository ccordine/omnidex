package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
	"github.com/jackc/pgx/v5/pgxpool"
)

type desiredExecutionFixture struct {
	repository *Repository
	pool       *pgxpool.Pool
	ctx        context.Context
	projectID  int64
	job        model.Job
	stepID     int64
	authority  model.StepAttemptAuthority
	graphID    string
	planID     string
	command    WorkspaceMutationCommand
	before     repositoryfacts.Snapshot
	after      repositoryfacts.Snapshot
}

func newDesiredExecutionFixture(t *testing.T, label, transition string) desiredExecutionFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "159")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	runQueueRepositoryGit(t, root, "init")
	runQueueRepositoryGit(t, root, "config", "user.email", "desired@example.test")
	runQueueRepositoryGit(t, root, "config", "user.name", "Desired Test")
	if err := os.WriteFile(filepath.Join(root, "value.go"), []byte("source-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	runQueueRepositoryGit(t, root, "add", "value.go")
	runQueueRepositoryGit(t, root, "commit", "-m", "source")
	before, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := workspacefacts.Capture(ctx, root)
	if err != nil || source.Git == nil || source.Git.RepositorySnapshotID != before.ID {
		t.Fatalf("capture desired workspace source: binding=%+v err=%v", source.Git, err)
	}
	project, err := repository.CreateProject(ctx, "desired-"+label, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.StoreRepositorySnapshot(ctx, project.ID, before); err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("desired-%s-%d", label, time.Now().UnixNano())
	metadata := []byte(fmt.Sprintf(`{"project_id":%d,"client_cwd":%q}`, project.ID, root))
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, metadata)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim desired execution job: claim=%+v err=%v", claim, err)
	}
	graphID := "desired_graph_" + queueTestSHA256("desired-execution-"+label)
	desired := desiredExecutionState(t, source, transition, []byte("desired-"+transition+"-post"))
	plan, err := workspacefacts.PlanMutation(ctx, source, graphID, []workspacefacts.DesiredFileState{desired})
	if err != nil {
		t.Fatal(err)
	}
	fixture := desiredExecutionFixture{
		repository: repository, pool: pool, ctx: ctx, projectID: project.ID,
		job: job, stepID: claim.Step.ID, authority: claim.Authority,
		graphID: graphID, planID: queueTestSHA256("one exact verification plan"),
		before: before,
	}
	fixture.command, fixture.after = fixture.executeMutation(t, source, plan)
	t.Cleanup(func() {
		_, _ = repository.CancelJob(context.Background(), testCancelCommand(
			t, job.ID, "desired-execution-cleanup", "close desired execution fixture",
		))
	})
	return fixture
}

func desiredExecutionState(
	t *testing.T,
	source workspacefacts.Snapshot,
	transition string,
	content []byte,
) workspacefacts.DesiredFileState {
	t.Helper()
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(append([]byte(nil), content...), '\n')
	}
	if transition == "create" {
		return workspacefacts.DesiredFileState{
			Path: "created.go", Present: true, Content: content, Mode: 0o644,
		}
	}
	var entry *workspacefacts.Entry
	for index := range source.Entries {
		if source.Entries[index].Path == "value.go" {
			entry = &source.Entries[index]
			break
		}
	}
	if entry == nil {
		t.Fatal("desired execution source lacks value.go")
	}
	exact := &workspacefacts.ExactSourceFile{
		EntryID: entry.ID, SHA256: entry.SHA256, Size: entry.Size, Mode: entry.Mode,
	}
	switch transition {
	case "modify":
		return workspacefacts.DesiredFileState{
			Path: entry.Path, Source: exact, Present: true, Content: content, Mode: entry.Mode,
		}
	case "delete":
		return workspacefacts.DesiredFileState{Path: entry.Path, Source: exact}
	default:
		t.Fatalf("unregistered desired execution transition %q", transition)
		return workspacefacts.DesiredFileState{}
	}
}

func (fixture desiredExecutionFixture) executeMutation(
	t *testing.T,
	source workspacefacts.Snapshot,
	plan workspacefacts.MutationPlan,
) (WorkspaceMutationCommand, repositoryfacts.Snapshot) {
	t.Helper()
	stage, err := workspacefacts.StageMutation(fixture.ctx, source, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stage.Cleanup(); err != nil {
			t.Error(err)
		}
	}()
	commandText := "verify desired repository state"
	verification, err := NewWorkspaceMutationVerificationPlan(
		[]WorkspaceMutationVerificationIntent{{Kind: evidence.KindTestResult, Command: commandText}},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := WorkspaceMutationCommand{
		JobID: fixture.job.ID, StepID: fixture.stepID,
		Generation:     fixture.authority.Generation,
		CreatorAttempt: fixture.authority.Attempt, CreatorWorkerID: fixture.authority.WorkerID,
		ProjectID: fixture.projectID, Plan: plan, Verification: verification,
	}
	var post repositoryfacts.Snapshot
	result, err := fixture.repository.ExecuteWorkspaceMutation(
		fixture.ctx, fixture.authority, command,
		WorkspaceMutationCallbacks{
			Observe: desiredMutationObserver,
			Apply: func(ctx context.Context, _ WorkspaceMutationCommand) error {
				_, applyErr := stage.ApplyVerified(ctx)
				return applyErr
			},
			Verify: func(ctx context.Context, exact WorkspaceMutationCommand) (WorkspaceMutationVerificationResult, error) {
				var verifyErr error
				post, verifyErr = repositoryfacts.BuildGitSnapshot(
					ctx, exact.Plan.WorkspaceRoot, repositoryfacts.SnapshotOptions{},
				)
				if verifyErr != nil {
					return WorkspaceMutationVerificationResult{}, verifyErr
				}
				if verifyErr = fixture.repository.StoreRepositorySnapshot(ctx, fixture.projectID, post); verifyErr != nil {
					return WorkspaceMutationVerificationResult{}, verifyErr
				}
				metadata := map[string]any{
					"repository_verification_scope":        "authoritative",
					"repository_mutation_owner_id":         fixture.graphID,
					"repository_desired_artifact_graph_id": fixture.graphID,
					"repository_source_snapshot_id":        exact.Plan.GitSourceSnapshotID,
					"workspace_source_snapshot_id":         exact.Plan.GitSourceSnapshotID,
					"workspace_mutation_stage_id":          exact.Plan.ID,
					"workspace_mutation_patch_sha256":      exact.Plan.PatchSHA256,
					"workspace_expected_state_id":          exact.Plan.ExpectedStateID,
					"repository_verification_plan_id":      fixture.planID,
					"repository_verification_snapshot_id":  post.ID,
					"repository_structured_proof_valid":    true,
					"succeeded":                            true,
				}
				return WorkspaceMutationVerificationResult{
					Succeeded: true, VerifiedRepositorySnapshotID: post.ID,
					CommandEvidence: []evidence.Record{{
						JobID: exact.JobID, StepID: exact.StepID, Kind: evidence.KindTestResult,
						Command: commandText, Confidence: 1, Metadata: metadata,
					}},
				}, nil
			},
		},
	)
	if err != nil || !result.VerificationSucceeded ||
		result.VerifiedRepositorySnapshotID != post.ID {
		t.Fatalf("execute desired workspace mutation: result=%+v err=%v", result, err)
	}
	return command, post
}

func desiredMutationObserver(
	ctx context.Context,
	exact WorkspaceMutationCommand,
) (WorkspaceMutationObservation, error) {
	current, err := workspacefacts.Capture(ctx, exact.Plan.WorkspaceRoot)
	if err != nil {
		return "", err
	}
	if current.ID == exact.Plan.SourceStateID {
		return WorkspaceMutationSource, nil
	}
	if exact.Plan.VerifyExpected(current) == nil {
		return WorkspaceMutationPost, nil
	}
	return WorkspaceMutationIndeterminate, nil
}

func (fixture desiredExecutionFixture) recordVerification(t *testing.T) {
	t.Helper()
	expectedPostID := queueTestSHA256("one exact expected post state")
	for _, scope := range []string{"baseline", "staged"} {
		metadata := map[string]any{
			"repository_verification_scope":        scope,
			"repository_mutation_owner_id":         fixture.graphID,
			"repository_desired_artifact_graph_id": fixture.graphID,
			"repository_source_snapshot_id":        fixture.before.ID,
			"repository_verification_plan_id":      fixture.planID,
		}
		if scope == "baseline" {
			metadata["repository_verification_baseline_id"] =
				"repository_baseline_" + queueTestSHA256("baseline")
		} else {
			metadata["repository_change_stage_id"] =
				"repository_change_stage_" + queueTestSHA256("staged")
			metadata["repository_change_patch_sha256"] = queueTestSHA256("staged-patch")
			metadata["repository_expected_post_id"] = expectedPostID
		}
		commandMetadata := cloneDesiredExecutionMetadata(metadata)
		commandMetadata["repository_structured_proof_valid"] = true
		commandMetadata["succeeded"] = true
		fixture.writeEvidence(t, evidence.Record{
			Kind: evidence.KindTestResult, SourceType: "command", SourceRef: "go",
			Command: "go test -json -count=1 ./...", Confidence: 1,
			Metadata: commandMetadata,
		})
		acceptanceMetadata := cloneDesiredExecutionMetadata(metadata)
		acceptanceMetadata["repository_verification_command_count"] = 1
		sourceType := "command-plan"
		if scope == "baseline" {
			sourceType = "command-baseline"
			acceptanceMetadata["repository_verification_baseline_accepted"] = true
		} else {
			acceptanceMetadata["repository_verification_plan_accepted"] = true
		}
		fixture.writeEvidence(t, evidence.Record{
			Kind: evidence.KindTestResult, SourceType: sourceType, SourceRef: "go",
			Confidence: 1, Metadata: acceptanceMetadata,
		})
	}
}

func (fixture desiredExecutionFixture) recordPostIndex(t *testing.T) {
	t.Helper()
	fixture.writeEvidence(t, evidence.Record{
		Kind: evidence.KindRepositoryIndex, SourceType: "repository",
		SourceRef: fixture.after.ID, Hash: fixture.after.GitStateSHA256, Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": fixture.after.ID, "file_count": len(fixture.after.Files),
			"analysis_ids": []string{},
		},
	})
}

func (fixture desiredExecutionFixture) writeEvidence(t *testing.T, record evidence.Record) {
	t.Helper()
	record.JobID, record.StepID = fixture.job.ID, fixture.stepID
	if err := fixture.repository.WriteEvidence(fixture.ctx, fixture.authority, record); err != nil {
		t.Fatal(fmt.Errorf("write desired execution evidence: %w", err))
	}
}

func cloneDesiredExecutionMetadata(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source)+2)
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
