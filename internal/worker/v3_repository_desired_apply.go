package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	goadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func (session *directCodingSession) runExistingRepositoryDesiredState(
	graph repositoryfacts.DesiredArtifactGraph,
	analysis repositoryfacts.Analysis,
) (string, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return "", fmt.Errorf("desired repository application requires one active indexed coding session")
	}
	before := *session.repositoryIndex
	if err := graph.Validate(before.Snapshot, analysis); err != nil {
		return "", fmt.Errorf("apply desired repository graph: %w", err)
	}
	commands, err := desiredArtifactGraphGoVerificationCommands(
		before.Snapshot, analysis, graph,
	)
	if err != nil {
		return "", err
	}
	baseline, err := session.proveExistingRepositoryBaseline(
		before.Snapshot, graph.ID, commands,
	)
	if err != nil {
		return "", err
	}
	generation, err := session.generateDesiredRepositoryDeclarations(graph)
	if err != nil {
		return "", err
	}
	compiled, err := compileDesiredRepositoryFileStates(
		graph, before.Snapshot, analysis, generation.Candidates,
	)
	if err != nil {
		return "", err
	}
	targetPaths, err := desiredRepositoryCompiledPaths(compiled)
	if err != nil {
		return "", err
	}
	// This proof deliberately runs before staging or authoritative mutation.
	// A user-supplied physical identity visible at any prior model boundary is
	// not silently relabeled as code-only authority.
	if _, err := session.proveDesiredRepositoryCalls(targetPaths); err != nil {
		return "", err
	}
	if err := baseline.RequireAuthority(before.Snapshot.ID, graph.ID, commands); err != nil {
		return "", fmt.Errorf("authorize desired repository mutation from clean baseline: %w", err)
	}
	result, err := session.executeDesiredRepositoryMutation(
		graph, analysis, commands, compiled, before,
	)
	if err != nil {
		return "", err
	}
	session.repositoryIndex = &result.Refreshed
	callProof, err := session.proveDesiredRepositoryCalls(targetPaths)
	if err != nil {
		return "", err
	}
	executionProof, err := session.runtime.svc.repo.DesiredRepositoryExecutionEvidence(
		session.runtime.ctx, session.runtime.claim.Authority, graph.ID,
		before.Snapshot.ID, result.Refreshed.Snapshot.ID,
	)
	if err != nil {
		return "", fmt.Errorf("prove desired repository execution from durable evidence: %w", err)
	}
	summary, err := desiredRepositoryMutationSummary(
		graph, commands, callProof, executionProof, compiled, before, result.Refreshed,
	)
	if err != nil {
		return "", err
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority, "repository_desired_state_verified",
		fmt.Sprintf("graph=%s files=%d snapshot=%s", graph.ID, len(result.ChangedFileIDs), result.Refreshed.Snapshot.ID),
	)
	return summary, nil
}

func (session *directCodingSession) executeDesiredRepositoryMutation(
	graph repositoryfacts.DesiredArtifactGraph,
	analysis repositoryfacts.Analysis,
	commands []testCommand,
	compiled desiredRepositoryCompileResult,
	before repositoryindex.Result,
) (existingRepositoryMutationResult, error) {
	return executeExistingRepositoryMutation(
		session.runtime.ctx, graph.ID, commands, before.Snapshot,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				return session.prepareVerifiedDesiredRepositoryState(
					before.Snapshot, analysis, graph, compiled.States, commands,
				)
			},
			mutate: func(ctx context.Context, prepared *verifiedRepositoryChangeStage) error {
				session.runtime.svc.emitStepEvent(
					session.runtime.claim.Authority, "repository_desired_state_staged",
					fmt.Sprintf("graph=%s files=%d", graph.ID, len(prepared.ChangedFileIDs())),
				)
				_, mutationErr := session.executeQueuedRepositoryWorkspaceMutation(
					ctx, graph.ID, commands, before.Snapshot, prepared,
				)
				return mutationErr
			},
			verifyAuthoritative: func(
				context.Context,
				*verifiedRepositoryChangeStage,
				[]testCommand,
			) error {
				return nil
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				return session.runtime.captureExistingRepositoryIndexWithAnalysis(
					session.root, goadapter.AdapterName,
				)
			},
		},
	)
}

func desiredRepositoryMutationSummary(
	graph repositoryfacts.DesiredArtifactGraph,
	commands []testCommand,
	callProof desiredRepositoryCallProof,
	executionProof queue.DesiredRepositoryExecutionEvidence,
	compiled desiredRepositoryCompileResult,
	before repositoryindex.Result,
	after repositoryindex.Result,
) (string, error) {
	if callProof.TotalModelCalls < 1 || callProof.SemanticGapCalls < 1 ||
		callProof.ModelSelectedMutationOperations != 0 || callProof.ModelVisibleTargetPaths != 0 {
		return "", fmt.Errorf("desired repository counters contain forbidden model authority")
	}
	mutationOperations, err := desiredRepositoryMutationOperations(compiled)
	if err != nil {
		return "", err
	}
	created, deleted := desiredRepositoryDeltaPaths(compiled)
	if len(created) != compiled.CreatedFiles || len(deleted) != compiled.DeletedFiles {
		return "", fmt.Errorf("desired repository inventory counters differ from compiled truth")
	}
	if executionProof.MutationOperations != 1 ||
		executionProof.FileTransitions != mutationOperations ||
		executionProof.CreatedFiles != len(created) ||
		executionProof.DeletedFiles != len(deleted) ||
		executionProof.ModifiedFiles != mutationOperations-len(created)-len(deleted) ||
		executionProof.VerificationCommands.Baseline != len(commands) ||
		executionProof.VerificationCommands.Staged != len(commands) ||
		executionProof.VerificationCommands.Authoritative != len(commands) ||
		executionProof.PostStateRepositoryReindexes != 1 ||
		executionProof.BeforeInventory != len(before.Snapshot.Files) ||
		executionProof.AfterInventory != len(after.Snapshot.Files) ||
		executionProof.InventoryDelta != len(after.Snapshot.Files)-len(before.Snapshot.Files) {
		return "", fmt.Errorf("desired repository durable execution counters differ from compiled truth")
	}
	labels := make([]string, len(commands))
	for index, command := range commands {
		labels[index] = directCodingCommandLabel(command)
	}
	return fmt.Sprintf(
		"Verified desired repository state: graph=%s deterministic_operations=%d model_calls_total=%d semantic_gap_calls=%d declaration_generation_calls=%d declaration_correction_calls=%d model_selected_mutation_operations=%d model_visible_target_paths=%d created_files=[%s] deleted_files=[%s] verification_commands=[%s] verification_command_executions=%d inventory_delta=%d snapshot=%s",
		graph.ID, executionProof.DeterministicOperations(), callProof.TotalModelCalls, callProof.SemanticGapCalls,
		callProof.DeclarationGenerationCalls, callProof.DeclarationCorrectionCalls,
		callProof.ModelSelectedMutationOperations, callProof.ModelVisibleTargetPaths,
		strings.Join(created, ","), strings.Join(deleted, ","), strings.Join(labels, ";"),
		executionProof.VerificationCommands.Total(), executionProof.InventoryDelta, after.Snapshot.ID,
	), nil
}

func desiredRepositoryCompiledPaths(compiled desiredRepositoryCompileResult) ([]string, error) {
	if _, err := desiredRepositoryMutationOperations(compiled); err != nil {
		return nil, err
	}
	paths := make([]string, len(compiled.States))
	for index, state := range compiled.States {
		paths[index] = state.Path
	}
	sort.Strings(paths)
	return paths, nil
}

func desiredRepositoryDeltaPaths(compiled desiredRepositoryCompileResult) ([]string, []string) {
	created := make([]string, 0, compiled.CreatedFiles)
	deleted := make([]string, 0, compiled.DeletedFiles)
	for _, state := range compiled.States {
		if state.Present {
			created = append(created, state.Path)
		} else {
			deleted = append(deleted, state.Path)
		}
	}
	sort.Strings(created)
	sort.Strings(deleted)
	return created, deleted
}
