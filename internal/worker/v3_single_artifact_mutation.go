package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (session *directCodingSession) applySinglePlainTextArtifact(
	requirement string,
	artifactPath string,
	content []byte,
	adapter directCodingArtifactAdapter,
) (summary string, resultErr error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil ||
		session.runtime.svc.repo == nil || session.repositoryIndex == nil {
		return "", fmt.Errorf("single-artifact mutation requires one active indexed session")
	}
	if err := validateDirectCodingArtifactSource(adapter, artifactPath, content); err != nil {
		return "", err
	}
	environment, err := newPlainTextProjectEnvironment(
		session.runtime, session.root, directCodingPlainTextEnvironmentSpec(),
	)
	if err != nil {
		return "", err
	}
	defer func() {
		resultErr = errors.Join(resultErr, environment.Close(context.Background()))
	}()
	if err := environment.Build(session.runtime.ctx); err != nil {
		return "", fmt.Errorf("build plain-text project environment: %w", err)
	}
	environmentAuthority, err := environment.Authority()
	if err != nil {
		return "", fmt.Errorf("capture plain-text project environment authority: %w", err)
	}
	if _, err := environment.Run(session.runtime.ctx, directCodingProjectEnvironmentCommand{
		Program: "test", Args: []string{"!", "-e", artifactPath}, Timeout: 30 * time.Second,
	}); err != nil {
		return "", fmt.Errorf("verify single-artifact target remains absent before mutation: %w", err)
	}

	source, err := workspacefacts.FromRepositorySnapshot(session.repositoryIndex.Snapshot)
	if err != nil {
		return "", err
	}
	ownerID := singleArtifactMutationOwnerID(requirement, session.repositoryIndex.Snapshot.ID)
	plan, err := workspacefacts.PlanMutation(
		session.runtime.ctx, source, ownerID,
		[]workspacefacts.DesiredFileState{{
			Path: artifactPath, Present: true, Content: append([]byte(nil), content...), Mode: 0o644,
		}},
	)
	if err != nil {
		return "", fmt.Errorf("plan single-artifact workspace mutation: %w", err)
	}
	stage, err := workspacefacts.StageMutation(session.runtime.ctx, source, plan)
	if err != nil {
		return "", fmt.Errorf("stage single-artifact workspace mutation: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, stage.Cleanup())
	}()
	commands := []testCommand{plainTextWorkspaceVerificationCommand(
		artifactPath, environmentAuthority,
	)}
	contentDigest := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(contentDigest[:])
	command, err := workspaceMutationCommandForStage(session.runtime, commands, stage)
	if err != nil {
		return "", err
	}
	result, err := session.runtime.svc.repo.ExecuteWorkspaceMutation(
		session.runtime.ctx,
		session.runtime.claim.Authority,
		command,
		queue.WorkspaceMutationCallbacks{
			Observe: observeWorkspaceMutation,
			Apply: func(ctx context.Context, _ queue.WorkspaceMutationCommand) error {
				_, applyErr := stage.ApplyVerified(ctx)
				return applyErr
			},
			Verify: func(
				ctx context.Context,
				exact queue.WorkspaceMutationCommand,
			) (queue.WorkspaceMutationVerificationResult, error) {
				return session.verifySinglePlainTextWorkspaceMutation(
					ctx, exact, commands, environment, artifactPath, expectedHash,
				)
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("execute single-artifact workspace mutation: %w", err)
	}
	if !result.VerificationSucceeded {
		return "", fmt.Errorf("single-artifact workspace verification failed")
	}
	if result.VerifiedRepositorySnapshotID == "" {
		return "", fmt.Errorf("single-artifact verification returned no persisted repository snapshot authority")
	}
	refreshed, err := session.runtime.captureExistingRepositoryIndex(session.root)
	if err != nil {
		return "", fmt.Errorf("capture verified single-artifact repository state: %w", err)
	}
	if refreshed.Snapshot.ID != result.VerifiedRepositorySnapshotID {
		return "", fmt.Errorf(
			"refreshed repository snapshot %s differs from verified authority %s",
			refreshed.Snapshot.ID, result.VerifiedRepositorySnapshotID,
		)
	}
	refreshedWorkspace, err := workspacefacts.FromRepositorySnapshot(refreshed.Snapshot)
	if err != nil {
		return "", err
	}
	if err := command.Plan.VerifyExpected(refreshedWorkspace); err != nil {
		return "", fmt.Errorf("refreshed repository state differs from complete mutation plan: %w", err)
	}
	if err := validateSinglePlainTextRepositoryPost(
		session.repositoryIndex.Snapshot, refreshed.Snapshot, artifactPath, content,
	); err != nil {
		return "", err
	}
	session.repositoryIndex = &refreshed
	session.completion.MutationCount++
	session.completion.WrittenSource[artifactPath] = string(content)
	session.mutationJournal = append(session.mutationJournal, directCodingMutationJournalEntry{
		Path: artifactPath, Operation: workspaceFileCreate,
	})
	return fmt.Sprintf(
		"Created and verified %s in the host-authoritative workspace.", artifactPath,
	), nil
}

func (session *directCodingSession) verifySinglePlainTextWorkspaceMutation(
	ctx context.Context,
	command queue.WorkspaceMutationCommand,
	commands []testCommand,
	environment *directCodingDockerProjectEnvironment,
	artifactPath string,
	expectedHash string,
) (queue.WorkspaceMutationVerificationResult, error) {
	if environment == nil || len(commands) != 1 {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf("plain-text verification requires one built environment and command")
	}
	if err := validateDirectCodingJournalCommands(command, commands); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	current, err := workspacefacts.Capture(ctx, command.Plan.WorkspaceRoot)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if err := command.Plan.VerifyExpected(current); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if current.Git == nil {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf("plain-text repository verification requires exact Git authority")
	}
	stored, err := session.runtime.captureExistingRepositoryIndex(command.Plan.WorkspaceRoot)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
			"persist verified repository snapshot before finalization: %w", err,
		)
	}
	if stored.Snapshot.ID != current.Git.RepositorySnapshotID {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
			"persisted repository snapshot %s differs from verified workspace snapshot %s",
			stored.Snapshot.ID, current.Git.RepositorySnapshotID,
		)
	}
	storedWorkspace, err := workspacefacts.FromRepositorySnapshot(stored.Snapshot)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if err := command.Plan.VerifyExpected(storedWorkspace); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
			"persisted repository snapshot differs from complete mutation plan: %w", err,
		)
	}
	environmentCommand, err := plainTextProjectEnvironmentCommand(commands[0])
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	execution, err := environment.Run(ctx, environmentCommand)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if !validRepositoryVerificationSHA256(expectedHash) {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf("plain-text verification requires one exact SHA-256 authority")
	}
	expectedOutput := expectedHash + "  " + artifactPath + "\n"
	if execution.Stdout != expectedOutput || execution.Stderr != "" ||
		execution.StdoutTruncated || execution.StderrTruncated {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
			"plain-text Docker digest output differs from exact artifact authority",
		)
	}
	encoded, err := encodeWorkspaceVerificationCommand(commands[0])
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	record := evidence.Record{
		JobID: command.JobID, StepID: command.StepID,
		Kind: evidence.KindTestResult, ToolName: "docker",
		Command: encoded, FilePaths: []string{artifactPath},
		Excerpt: execution.Stdout,
		Summary: "Docker project environment verified the exact host-bound artifact digest.",
		Hash:    expectedHash, Confidence: 1,
		Metadata: map[string]any{
			"succeeded":                            true,
			"project_environment_id":               directCodingPlainTextEnvironmentID,
			"project_environment_image_id":         environment.ImageID(),
			"project_environment_authority_sha256": commands[0].Environment.AuthoritySHA256,
			"workspace_verification_role":          string(workspaceVerificationPrimary),
			"expected_sha256":                      expectedHash,
			"exit_code":                            execution.ExitCode,
		},
	}
	return queue.WorkspaceMutationVerificationResult{
		Succeeded: true, CommandEvidence: []evidence.Record{record},
		VerifiedRepositorySnapshotID: stored.Snapshot.ID,
	}, nil
}

func validateSinglePlainTextRepositoryPost(
	before repositoryfacts.Snapshot,
	after repositoryfacts.Snapshot,
	artifactPath string,
	content []byte,
) error {
	if err := before.Validate(); err != nil {
		return err
	}
	if err := after.Validate(); err != nil {
		return err
	}
	digest := sha256.Sum256(content)
	want := hex.EncodeToString(digest[:])
	found := false
	for _, file := range after.Files {
		if file.Path != artifactPath {
			continue
		}
		if found || file.Kind != repositoryfacts.EntryRegular || file.SHA256 != want ||
			file.Size != int64(len(content)) || file.Mode != 0o644 {
			return fmt.Errorf("verified single-artifact repository post-image differs from exact content")
		}
		found = true
	}
	if !found {
		return fmt.Errorf("verified single-artifact repository post-image lacks %q", artifactPath)
	}
	for _, file := range before.Files {
		if file.Path == artifactPath {
			return fmt.Errorf("single-artifact creation target %q existed in source authority", artifactPath)
		}
	}
	if before.RepositoryID != after.RepositoryID || before.HeadCommit != after.HeadCommit {
		return fmt.Errorf("single-artifact creation changed repository identity or Git HEAD")
	}
	return nil
}

func singlePlainTextVerificationFamily(commands []testCommand) bool {
	if len(commands) != 1 {
		return false
	}
	return strings.TrimSpace(commands[0].Family) == plainTextWorkspaceVerificationFamily
}

func singlePlainTextMutationAuthority(
	command queue.WorkspaceMutationCommand,
	commands []testCommand,
) (artifactPath string, expectedHash string, err error) {
	if !singlePlainTextVerificationFamily(commands) || len(command.Plan.Files) != 1 {
		return "", "", fmt.Errorf("plain-text workspace mutation requires one command and file transition")
	}
	transition := command.Plan.Files[0]
	if transition.Source.Present || !transition.Expected.Present ||
		transition.Expected.Mode != 0o644 || transition.Expected.Size < 1 ||
		!validRepositoryVerificationSHA256(transition.Expected.SHA256) ||
		len(commands[0].Args) != 1 || commands[0].Args[0] != transition.Path {
		return "", "", fmt.Errorf("plain-text workspace mutation authority is not one exact creation")
	}
	if err := validatePlainTextWorkspaceVerificationCommand(commands[0]); err != nil {
		return "", "", err
	}
	return transition.Path, transition.Expected.SHA256, nil
}
