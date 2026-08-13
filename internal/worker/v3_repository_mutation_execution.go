package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

type existingRepositoryMutationExecution struct {
	prepare             func(context.Context) (*verifiedRepositoryChangeStage, error)
	mutate              func(context.Context, *verifiedRepositoryChangeStage) error
	verifyAuthoritative func(context.Context, *verifiedRepositoryChangeStage, []testCommand) error
	refresh             func(context.Context) (repositoryindex.Result, error)
}

type existingRepositoryMutationResult struct {
	StageID        string
	ChangedFileIDs []string
	Refreshed      repositoryindex.Result
}

func executeExistingRepositoryMutation(
	ctx context.Context,
	contractID string,
	commands []testCommand,
	before repositoryfacts.Snapshot,
	execution existingRepositoryMutationExecution,
) (result existingRepositoryMutationResult, resultErr error) {
	if ctx == nil {
		return result, fmt.Errorf("repository mutation execution requires a context")
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("repository mutation execution: %w", err)
	}
	if strings.TrimSpace(contractID) == "" {
		return result, fmt.Errorf("repository mutation execution requires one exact contract")
	}
	if err := before.Validate(); err != nil {
		return result, fmt.Errorf("repository mutation execution source snapshot: %w", err)
	}
	if err := validateRepositoryGoVerificationPlan(repositoryVerificationStaged, commands); err != nil {
		return result, fmt.Errorf("repository mutation execution verification plan: %w", err)
	}
	if execution.prepare == nil || execution.mutate == nil ||
		execution.verifyAuthoritative == nil || execution.refresh == nil {
		return result, fmt.Errorf("repository mutation execution requires prepare, mutate, authoritative proof, and refresh authority")
	}
	prepared, err := execution.prepare(ctx)
	if err != nil {
		return result, fmt.Errorf("prepare verified repository change: %w", err)
	}
	if prepared == nil {
		return result, fmt.Errorf("prepare verified repository change returned no stage")
	}
	defer func() {
		if cleanupErr := prepared.Cleanup(); cleanupErr != nil {
			result = existingRepositoryMutationResult{}
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	if err := prepared.RequireAuthority(contractID, commands); err != nil {
		return result, err
	}
	if err := validateVerifiedRepositoryChangeStage(prepared); err != nil {
		return result, err
	}
	if err := execution.mutate(ctx, prepared); err != nil {
		return result, fmt.Errorf("apply queue-authoritative repository mutation: %w", err)
	}
	verificationErr := execution.verifyAuthoritative(ctx, prepared, cloneTestCommands(commands))
	refreshed, refreshErr := execution.refresh(ctx)
	if refreshErr == nil {
		refreshErr = validateRepositoryRefreshAuthority(refreshed)
	}
	if refreshErr == nil {
		refreshErr = validateRefreshedRepositoryChange(before, refreshed, prepared.ExpectedFiles())
	}
	if verificationErr != nil || refreshErr != nil {
		return result, errors.Join(
			wrapRepositoryExecutionError("authoritative repository proof", verificationErr),
			wrapRepositoryExecutionError("refresh authoritative repository index", refreshErr),
		)
	}
	result = existingRepositoryMutationResult{
		StageID: prepared.ID(), ChangedFileIDs: prepared.ChangedFileIDs(), Refreshed: refreshed,
	}
	return result, nil
}

func cloneTestCommands(commands []testCommand) []testCommand {
	cloned := make([]testCommand, len(commands))
	for index, command := range commands {
		cloned[index] = command
		cloned[index].Args = append([]string(nil), command.Args...)
		if command.RepositoryProof != nil {
			proof := *command.RepositoryProof
			proof.Expected = append([]repositoryGoExpectedTest(nil), command.RepositoryProof.Expected...)
			for expected := range proof.Expected {
				proof.Expected[expected].TargetSymbolIDs = append(
					[]string(nil), proof.Expected[expected].TargetSymbolIDs...,
				)
			}
			cloned[index].RepositoryProof = &proof
		}
	}
	return cloned
}

func validateVerifiedRepositoryChangeStage(prepared *verifiedRepositoryChangeStage) error {
	if prepared == nil || prepared.ID() == "" {
		return fmt.Errorf("verified repository change stage has no exact identity")
	}
	patch := []byte(prepared.Patch())
	if len(patch) == 0 || len(patch) > maxRepositoryPatchEvidenceBytes {
		return fmt.Errorf(
			"repository change patch has %d bytes; exact evidence requires 1-%d bytes",
			len(patch), maxRepositoryPatchEvidenceBytes,
		)
	}
	digest := sha256.Sum256(patch)
	if prepared.PatchSHA256() != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("verified repository change stage patch identity is invalid")
	}
	changed := prepared.ChangedFileIDs()
	expected := prepared.ExpectedFiles()
	if len(changed) == 0 || len(changed) != len(expected) {
		return fmt.Errorf("verified repository change stage has incomplete changed-file authority")
	}
	if _, err := canonicalExpectedRepositoryFileStates(changed, expected); err != nil {
		return fmt.Errorf("verified repository change stage has invalid exact post-file authority: %w", err)
	}
	return nil
}

func validateRepositoryRefreshAuthority(refreshed repositoryindex.Result) error {
	if !refreshed.Complete {
		return fmt.Errorf("refreshed repository index is incomplete")
	}
	if err := refreshed.Snapshot.Validate(); err != nil {
		return fmt.Errorf("refreshed repository snapshot authority: %w", err)
	}
	if len(refreshed.Analyses) == 0 {
		return fmt.Errorf("refreshed repository index has no complete analysis authority")
	}
	seen := make(map[string]struct{}, len(refreshed.Analyses))
	for _, analysis := range refreshed.Analyses {
		if _, duplicate := seen[analysis.ID]; duplicate {
			return fmt.Errorf("refreshed repository analysis authority is duplicated")
		}
		seen[analysis.ID] = struct{}{}
		if err := analysis.Validate(refreshed.Snapshot); err != nil {
			return fmt.Errorf("refreshed repository analysis authority: %w", err)
		}
		if !analysis.Complete {
			return fmt.Errorf("refreshed repository analysis %q is incomplete", analysis.ID)
		}
	}
	return nil
}

func wrapRepositoryExecutionError(subject string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", subject, err)
}
