package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func stageWorkspaceMutationFromRepositoryChange(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	ownerID string,
	commands []testCommand,
	prepared *verifiedRepositoryChangeStage,
) (*workspacefacts.StagedMutation, error) {
	if ctx == nil || prepared == nil {
		return nil, fmt.Errorf("workspace mutation staging requires a context and verified repository stage")
	}
	if err := prepared.RequireAuthority(ownerID, commands); err != nil {
		return nil, err
	}
	if err := prepared.stage.VerifyExactDelta(ctx); err != nil {
		return nil, fmt.Errorf("verify repository semantic delta: %w", err)
	}
	source, err := workspacefacts.FromRepositorySnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	desired, err := workspaceDesiredStatesFromRepositoryStage(prepared, source)
	if err != nil {
		return nil, err
	}
	plan, err := workspacefacts.PlanMutation(ctx, source, ownerID, desired)
	if err != nil {
		return nil, fmt.Errorf("plan generic workspace mutation: %w", err)
	}
	stage, err := workspacefacts.StageMutation(ctx, source, plan)
	if err != nil {
		return nil, fmt.Errorf("stage generic workspace mutation: %w", err)
	}
	return stage, nil
}

func workspaceDesiredStatesFromRepositoryStage(
	prepared *verifiedRepositoryChangeStage,
	source workspacefacts.Snapshot,
) ([]workspacefacts.DesiredFileState, error) {
	entries := make(map[string]workspacefacts.Entry, len(source.Entries))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	expected := prepared.ExpectedFiles()
	desired := make([]workspacefacts.DesiredFileState, len(expected))
	for index, state := range expected {
		entry, present := entries[state.Path]
		var exactSource *workspacefacts.ExactSourceFile
		if present {
			exactSource = &workspacefacts.ExactSourceFile{
				EntryID: entry.ID, SHA256: entry.SHA256,
				Size: entry.Size, Mode: entry.Mode,
			}
		}
		desired[index] = workspacefacts.DesiredFileState{
			Path: state.Path, Source: exactSource,
			Present: state.Present, Mode: state.Mode,
		}
		if !state.Present {
			continue
		}
		absolute := filepath.Join(prepared.stage.DeltaRoot(), filepath.FromSlash(state.Path))
		info, err := os.Lstat(absolute)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != os.FileMode(state.Mode) ||
			info.Size() != state.Size {
			return nil, fmt.Errorf("repository semantic delta post-image %q differs from exact state", state.Path)
		}
		content, err := os.ReadFile(absolute)
		if err != nil {
			return nil, fmt.Errorf("read repository semantic delta post-image %q: %w", state.Path, err)
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != state.SHA256 {
			return nil, fmt.Errorf("repository semantic delta post-image %q has a different digest", state.Path)
		}
		desired[index].Content = content
	}
	return desired, nil
}

func workspaceMutationCommandForStage(
	runtime *nativeRuntimeV3,
	commands []testCommand,
	stage *workspacefacts.StagedMutation,
) (queue.WorkspaceMutationCommand, error) {
	if runtime == nil || runtime.claim == nil || runtime.svc == nil ||
		runtime.svc.repo == nil || stage == nil {
		return queue.WorkspaceMutationCommand{}, fmt.Errorf("workspace mutation command requires one active claim and stage")
	}
	intents := make([]queue.WorkspaceMutationVerificationIntent, len(commands))
	for index, command := range commands {
		authority, err := encodeWorkspaceVerificationCommand(command)
		if err != nil {
			return queue.WorkspaceMutationCommand{}, fmt.Errorf(
				"encode workspace verification command %d: %w", index+1, err,
			)
		}
		kind := workspaceVerificationEvidenceKind(command.Purpose)
		intents[index] = queue.WorkspaceMutationVerificationIntent{Kind: kind, Command: authority}
	}
	verification, err := queue.NewWorkspaceMutationVerificationPlan(intents)
	if err != nil {
		return queue.WorkspaceMutationCommand{}, err
	}
	claim := runtime.claim
	projectID, err := runtime.svc.repo.JobProjectID(runtime.ctx, claim.Job.ID)
	if err != nil {
		return queue.WorkspaceMutationCommand{}, fmt.Errorf("resolve workspace mutation project: %w", err)
	}
	project, err := runtime.svc.repo.GetProject(runtime.ctx, projectID)
	if err != nil {
		return queue.WorkspaceMutationCommand{}, fmt.Errorf("load workspace mutation project authority: %w", err)
	}
	plan := stage.Plan()
	if err := requireWorkspaceMutationStageAuthority(
		runtime.svc, claim.Job, projectID, project, plan,
	); err != nil {
		return queue.WorkspaceMutationCommand{}, err
	}
	return queue.WorkspaceMutationCommand{
		JobID: claim.Authority.JobID, StepID: claim.Authority.StepID,
		Generation:      claim.Authority.Generation,
		CreatorAttempt:  claim.Authority.Attempt,
		CreatorWorkerID: claim.Authority.WorkerID,
		ProjectID:       projectID,
		ProjectLocation: project.Location,
		Plan:            plan, Verification: verification,
	}, nil
}

func requireWorkspaceMutationStageAuthority(
	service *Service,
	job model.Job,
	projectID int64,
	project model.Project,
	plan workspacefacts.MutationPlan,
) error {
	if service == nil || projectID <= 0 || project.ID != projectID {
		return fmt.Errorf("workspace mutation project authority is incomplete")
	}
	jobLocation := strings.TrimSpace(codingWorkspaceForJob(job))
	if err := model.ValidateChannelWorkspaceRoot(jobLocation); err != nil {
		return fmt.Errorf("workspace mutation project locations must be absolute")
	}
	if err := model.ValidateChannelWorkspaceRoot(project.Location); err != nil ||
		project.Location == "/" || strings.ContainsAny(project.Location, "\\\r\n") {
		return fmt.Errorf("workspace mutation current project location is not canonical")
	}
	jobLocation = filepath.Clean(jobLocation)
	projectLocation := filepath.Clean(project.Location)
	if projectLocation != jobLocation {
		return fmt.Errorf("workspace mutation job location differs from its current project")
	}
	scope, err := service.workspaceScopeForV3Job(job)
	if err != nil {
		return fmt.Errorf("resolve workspace mutation runtime root: %w", err)
	}
	if plan.WorkspaceRoot != scope.Root {
		return fmt.Errorf("workspace mutation plan root differs from its resolved runtime root")
	}
	return nil
}

func observeWorkspaceMutation(
	ctx context.Context,
	command queue.WorkspaceMutationCommand,
) (queue.WorkspaceMutationObservation, error) {
	current, err := workspacefacts.Capture(ctx, command.Plan.WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("capture workspace mutation observation: %w", err)
	}
	if err := command.Plan.ValidateSource(current); err == nil {
		return queue.WorkspaceMutationSource, nil
	}
	if err := command.Plan.VerifyExpected(current); err == nil {
		return queue.WorkspaceMutationPost, nil
	}
	return queue.WorkspaceMutationIndeterminate, nil
}
