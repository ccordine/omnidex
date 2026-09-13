package worker

import (
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

type directCodingPreparedMutation struct {
	reconciliation           workspacefacts.Prepared
	result                   workspacefacts.ReconciliationResult
	hostVerificationProgram  *directCodingProgram
	hostVerificationAssembly directCodingAssembly
	expectedAssembly         directCodingAssembly
}

func (s *directCodingSession) PrepareAssembly(
	assembly directCodingAssembly,
) (*directCodingPreparedMutation, error) {
	program, programAssembly, verify, err := s.hostVerificationAuthority(assembly)
	if err != nil {
		return nil, err
	}
	prepared, err := s.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		return nil, err
	}
	if verify {
		prepared.hostVerificationProgram = program
		prepared.hostVerificationAssembly = programAssembly
	}
	return prepared, nil
}

func (s *directCodingSession) prepareWorkspaceReconciliation(
	assembly directCodingAssembly,
) (*directCodingPreparedMutation, error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || s.runtime.svc == nil {
		return nil, fmt.Errorf("workspace mutation preparation requires one active session")
	}
	desired, err := s.directCodingAssemblyDesiredStates(assembly)
	if err != nil {
		return nil, err
	}
	// Desired-state validation may remove a move optimization to preserve
	// its protected source. Verification consumes that same effective state.
	assembly = cloneDirectCodingAssembly(assembly)
	files := make(map[string]int, len(assembly.Files))
	for index, file := range assembly.Files {
		files[file.Path] = index
	}
	for _, state := range desired {
		if index, exists := files[state.Path]; exists {
			assembly.Files[index].MoveFrom = state.MoveFrom
		}
	}
	unpublished, err := directCodingUnpublishedAssembly(assembly, s.publishedAssembly)
	if err != nil {
		return nil, err
	}
	if s.program != nil && s.program.Project.Stack.ID == genericTypeScriptBrowserAdapter {
		if err := validateDirectCodingTypeScriptGreenfieldAssembly(s.runtime.ctx, s.runtime.workspaceFence, unpublished); err != nil {
			return nil, err
		}
	}
	s.plannedFiles = 0
	s.plannedDeletes = 0
	for _, state := range desired {
		if state.Present {
			s.plannedFiles++
		} else {
			s.plannedDeletes++
		}
	}
	// Already observed files are verified below, never rewritten by later
	// publication. Only newly ready files have mutation authority.
	retained := make(map[string]struct{}, len(s.publishedAssembly.Files))
	for _, file := range s.publishedAssembly.Files {
		retained[file.Path] = struct{}{}
	}
	mutations := desired[:0]
	for _, state := range desired {
		_, previous := retained[state.Path]
		if !previous {
			mutations = append(mutations, state)
		}
	}
	if err := s.runtime.svc.requireWorkspaceScopeForV3Job(
		s.runtime.ctx, s.runtime.claim.Job,
		s.root,
	); err != nil {
		return nil, fmt.Errorf("validate host workspace before mutation preparation: %w", err)
	}
	if err := s.runtime.requireWorkspaceMutationFence(); err != nil {
		return nil, err
	}
	if err := validateDirectCodingAssembly(s.runtime.ctx, s.runtime.workspaceFence, s.publishedAssembly); err != nil {
		return nil, fmt.Errorf("previously published workspace changed: %w", err)
	}
	reconciliation, err := s.runtime.workspaceFence.Prepare(
		s.runtime.ctx,
		mutations,
		directCodingExpectedWorkspaceFiles(s.publishedAssembly),
	)
	if err != nil {
		return nil, fmt.Errorf("prepare direct-coding workspace reconciliation: %w", err)
	}
	return &directCodingPreparedMutation{
		reconciliation: reconciliation, expectedAssembly: assembly,
	}, nil
}

func directCodingExpectedWorkspaceFiles(assembly directCodingAssembly) []workspacefacts.File {
	files := make([]workspacefacts.File, len(assembly.Files))
	for index, file := range assembly.Files {
		files[index] = workspacefacts.File{
			Entry:   workspacefacts.Entry{Path: file.Path, Kind: workspacefacts.EntryFile, Mode: file.Mode, Size: int64(len(file.Content))},
			Content: []byte(file.Content),
		}
	}
	return files
}

func (s *directCodingSession) directCodingAssemblyDesiredStates(
	assembly directCodingAssembly,
) ([]workspacefacts.DesiredFile, error) {
	desired := make([]workspacefacts.DesiredFile, 0, len(assembly.Files)+len(assembly.RequiredPaths)+len(assembly.DeletePaths))
	exactFiles := make(map[string]struct{}, len(assembly.Files))
	desiredPaths := make(map[string]struct{}, len(assembly.Files)+len(assembly.RequiredPaths)+len(assembly.DeletePaths))
	deletions := make(map[string]struct{}, len(assembly.DeletePaths))
	createOnly := s.program != nil &&
		s.program.Project.Stack.ID == genericTypeScriptBrowserAdapter
	published := make(map[string]struct{}, len(s.publishedAssembly.Files))
	for _, file := range s.publishedAssembly.Files {
		published[file.Path] = struct{}{}
	}
	for _, path := range assembly.DeletePaths {
		deletions[path] = struct{}{}
	}
	for _, task := range assembly.Files {
		if directCodingPathProtected(task.Path, s.protectedPaths) {
			return nil, fmt.Errorf(
				"compiled file %q conflicts with accepted preservation authority", task.Path,
			)
		}
		for deletion := range deletions {
			if task.Path == deletion || directCodingTargetTreeFileAncestor(deletion, task.Path) {
				return nil, fmt.Errorf(
					"compiled file %q conflicts with accepted deletion %q", task.Path, deletion,
				)
			}
		}
		_, alreadyPublished := published[task.Path]
		state := workspacefacts.DesiredFile{
			Path: task.Path, Present: true,
			Content: append([]byte(nil), task.Content...), Mode: task.Mode,
			MoveFrom: task.MoveFrom, CreateOnly: createOnly && !alreadyPublished,
		}
		if state.MoveFrom != "" && directCodingPathProtected(state.MoveFrom, s.protectedPaths) {
			state.MoveFrom = ""
		}
		desired = append(desired, state)
		exactFiles[task.Path] = struct{}{}
		desiredPaths[task.Path] = struct{}{}
	}
	for _, required := range assembly.RequiredPaths {
		if _, exists := exactFiles[required]; exists {
			continue
		}
		if directCodingPathProtected(required, s.protectedPaths) {
			return nil, fmt.Errorf(
				"required file %q conflicts with accepted preservation authority", required,
			)
		}
		for deletion := range deletions {
			if directCodingTargetTreeFileHierarchyConflict(required, deletion) {
				return nil, fmt.Errorf(
					"required file %q conflicts with accepted deletion %q", required, deletion,
				)
			}
		}
		if directCodingPathConflictsWithSet(required, desiredPaths) {
			return nil, fmt.Errorf(
				"required file %q crosses an accepted file hierarchy", required,
			)
		}
		desired = append(desired, workspacefacts.DesiredFile{
			Path: required, Present: true, Content: []byte{}, Mode: 0o644,
			PreserveExisting: true,
		})
		desiredPaths[required] = struct{}{}
	}
	for _, path := range assembly.DeletePaths {
		if directCodingPathProtected(path, s.protectedPaths) {
			return nil, fmt.Errorf(
				"deleted file %q conflicts with accepted preservation authority", path,
			)
		}
		redundant := false
		for desiredPath := range desiredPaths {
			if !directCodingTargetTreeFileHierarchyConflict(path, desiredPath) {
				continue
			}
			if directCodingTargetTreeFileAncestor(desiredPath, path) {
				redundant = true
				break
			}
			return nil, fmt.Errorf(
				"deleted file %q conflicts with accepted file %q", path, desiredPath,
			)
		}
		if redundant {
			continue
		}
		desired = append(desired, workspacefacts.DesiredFile{Path: path})
		desiredPaths[path] = struct{}{}
	}
	return desired, nil
}

func directCodingPathConflictsWithSet(candidate string, paths map[string]struct{}) bool {
	for existing := range paths {
		if directCodingTargetTreeFileHierarchyConflict(candidate, existing) {
			return true
		}
	}
	return false
}
