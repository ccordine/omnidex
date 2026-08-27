package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

type directCodingPreparedMutation struct {
	assembly        directCodingAssembly
	stage           *workspacefacts.StagedMutation
	command         queue.WorkspaceMutationCommand
	projection      repositoryWorkspaceProjection
	primaryCommands []testCommand
	allCommands     []testCommand
	mutationCount   int
	recorded        bool
}

func (prepared *directCodingPreparedMutation) MutationCount() int {
	if prepared == nil {
		return 0
	}
	return prepared.mutationCount
}

func (prepared *directCodingPreparedMutation) Cleanup() error {
	if prepared == nil || prepared.stage == nil {
		return nil
	}
	err := prepared.stage.Cleanup()
	if err == nil {
		prepared.stage = nil
	}
	return err
}

func (s *directCodingSession) PrepareAssembly(
	assembly directCodingAssembly,
) (_ *directCodingPreparedMutation, resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || s.cognition == nil ||
		s.specification == nil || s.program == nil {
		return nil, fmt.Errorf("workspace mutation preparation requires one compiled direct-coding session")
	}
	if err := assembly.validate(); err != nil {
		return nil, err
	}
	if diagnostic := directCodingAssemblyUnfinishedDiagnostic(assembly); diagnostic != nil {
		return nil, fmt.Errorf("%s: %s", diagnostic.Stage, diagnostic.Detail)
	}
	source, err := workspacefacts.Capture(s.runtime.ctx, s.root)
	if err != nil {
		return nil, fmt.Errorf("capture direct-coding workspace: %w", err)
	}
	if err := validateDirectCodingAssemblyDirectories(source, assembly); err != nil {
		return nil, err
	}
	desired, err := s.directCodingAssemblyDesiredStates(source, assembly)
	if err != nil {
		return nil, err
	}
	ownerID, err := directCodingAssemblyOwnerID(assembly)
	if err != nil {
		return nil, err
	}
	plan, err := workspacefacts.PlanMutation(s.runtime.ctx, source, ownerID, desired)
	if err != nil {
		return nil, fmt.Errorf("plan direct-coding workspace mutation: %w", err)
	}
	stage, err := workspacefacts.StageMutation(s.runtime.ctx, source, plan)
	if err != nil {
		return nil, fmt.Errorf("stage direct-coding workspace mutation: %w", err)
	}
	prepared := &directCodingPreparedMutation{assembly: assembly, stage: stage}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, prepared.Cleanup())
		}
	}()
	prepared.primaryCommands, prepared.allCommands, err = s.directCodingJournalCommands()
	if err != nil {
		return nil, err
	}
	prepared.command, err = workspaceMutationCommandForStage(
		s.runtime, prepared.allCommands, stage,
	)
	if err != nil {
		return nil, err
	}
	prepared.mutationCount = len(prepared.command.Plan.Files)
	prepared.projection, err = newWorkspaceStagedProjection(s.runtime.ctx, stage)
	if err != nil {
		return nil, fmt.Errorf("project direct-coding staged workspace: %w", err)
	}
	return prepared, nil
}

func (s *directCodingSession) directCodingAssemblyDesiredStates(
	source workspacefacts.Snapshot,
	assembly directCodingAssembly,
) ([]workspacefacts.DesiredFileState, error) {
	if len(assembly.Files)+len(assembly.DeletePaths) > workspacefacts.MaxMutationFiles {
		return nil, fmt.Errorf(
			"direct-coding workspace delta exceeds the %d-file transaction limit",
			workspacefacts.MaxMutationFiles,
		)
	}
	entries := make(map[string]workspacefacts.Entry, len(source.Entries))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	desired := make([]workspacefacts.DesiredFileState, 0, len(assembly.Files)+len(assembly.DeletePaths))
	for _, task := range assembly.Files {
		if err := rejectDirectCodingProtectedMutation(task.Path, s.protectedPaths); err != nil {
			return nil, err
		}
		if err := s.validateProgramSource(task.Path, task.Content); err != nil {
			return nil, err
		}
		state := workspacefacts.DesiredFileState{
			Path: task.Path, Present: true, Content: []byte(task.Content), Mode: 0o644,
		}
		if entry, exists := entries[task.Path]; exists {
			state.Source = exactDirectCodingWorkspaceSource(entry)
			state.Mode = entry.Mode
		}
		desired = append(desired, state)
	}
	for _, path := range assembly.DeletePaths {
		if err := rejectDirectCodingProtectedMutation(path, s.protectedPaths); err != nil {
			return nil, err
		}
		state := workspacefacts.DesiredFileState{Path: path}
		if entry, exists := entries[path]; exists {
			state.Source = exactDirectCodingWorkspaceSource(entry)
		}
		desired = append(desired, state)
	}
	return desired, nil
}

func exactDirectCodingWorkspaceSource(
	entry workspacefacts.Entry,
) *workspacefacts.ExactSourceFile {
	return &workspacefacts.ExactSourceFile{
		EntryID: entry.ID, SHA256: entry.SHA256, Size: entry.Size, Mode: entry.Mode,
	}
}

func validateDirectCodingAssemblyDirectories(
	source workspacefacts.Snapshot,
	assembly directCodingAssembly,
) error {
	for _, relative := range assembly.Directories {
		absolute := filepath.Join(source.Root, filepath.FromSlash(relative))
		info, err := os.Lstat(absolute)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("direct-coding directory %q has invalid existing authority", relative)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect direct-coding directory %q: %w", relative, err)
		}
		derived := false
		prefix := relative + "/"
		for _, file := range assembly.Files {
			if strings.HasPrefix(file.Path, prefix) {
				derived = true
				break
			}
		}
		if !derived {
			return fmt.Errorf(
				"direct-coding directory %q has no file state from which code can derive it",
				relative,
			)
		}
	}
	return nil
}

func directCodingAssemblyOwnerID(assembly directCodingAssembly) (string, error) {
	raw, err := json.Marshal(assembly)
	if err != nil {
		return "", fmt.Errorf("encode direct-coding assembly identity: %w", err)
	}
	return "coding_" + directCodingDigest(string(raw)), nil
}

func directCodingAssemblyUnfinishedDiagnostic(
	assembly directCodingAssembly,
) *directCodingDiagnostic {
	written := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		written[file.Path] = file.Content
	}
	return directCodingUnfinishedDiagnostic(directCodingCompletionState{WrittenSource: written})
}

func (s *directCodingSession) directCodingJournalCommands() (
	[]testCommand,
	[]testCommand,
	error,
) {
	primary, err := directCodingProgramVerificationCommands(*s.specification, *s.program)
	if err != nil {
		return nil, nil, err
	}
	stack, err := directCodingProjectStackByID(s.program.StackID)
	if err != nil {
		return nil, nil, err
	}
	primary = cloneWorkspaceRoleCommands(primary, workspaceVerificationPrimary)
	cleanup := cloneWorkspaceRoleCommands(stack.CleanupCommands, workspaceVerificationCleanup)
	all := append(append([]testCommand(nil), primary...), cleanup...)
	if len(all) > 32 {
		return nil, nil, fmt.Errorf("direct-coding verification and cleanup exceed the 32-command journal limit")
	}
	return primary, all, nil
}

func cloneWorkspaceRoleCommands(
	commands []testCommand,
	role workspaceVerificationCommandRole,
) []testCommand {
	cloned := make([]testCommand, len(commands))
	for index, command := range commands {
		command.Args = append([]string(nil), command.Args...)
		command.RepositoryProof = cloneRepositoryGoTestProof(command.RepositoryProof)
		command.WorkspaceRole = role
		cloned[index] = command
	}
	return cloned
}

func (s *directCodingSession) persistCodeOwnedEvidenceIDs(
	result operation.Result,
) ([]int64, error) {
	if len(result.Evidence) == 0 {
		return nil, fmt.Errorf("code-owned operation produced no evidence")
	}
	ids := make([]int64, 0, len(result.Evidence))
	for _, record := range result.Evidence {
		id, err := s.runtime.writeEvidenceReturningID(record)
		if err != nil {
			return nil, err
		}
		if id <= 0 || len(ids) > 0 && id <= ids[len(ids)-1] {
			return nil, fmt.Errorf("code-owned evidence identities are not strictly increasing")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
