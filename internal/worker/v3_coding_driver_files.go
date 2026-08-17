package worker

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/operation"
)

func (s *directCodingSession) EnsureDirectory(path string) (bool, error) {
	if s.cognition == nil {
		return false, fmt.Errorf("directory materialization requires persisted task cognition")
	}
	if err := s.cognition.BeginTreeDirectory(path); err != nil {
		return false, err
	}
	target, err := resolveV3WorkspaceFile(s.root, path)
	if err != nil {
		return false, err
	}
	changed := false
	if info, statErr := os.Stat(target); statErr == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("ensure coding directory %s: existing target is not a directory", path)
		}
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("inspect coding directory %s: %w", path, statErr)
	} else {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return false, fmt.Errorf("ensure coding directory %s: %w", path, err)
		}
		changed = true
		s.recordEnsuredDirectory(path)
	}
	if err := s.cognition.CompleteTreeDirectory(path, "directory="+path+" state="+map[bool]string{true: "created", false: "present"}[changed]); err != nil {
		return false, err
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_directory_ensured", fmt.Sprintf(
		"path=%s state=%s", safeLine(path, "unknown"), map[bool]string{true: "created", false: "present"}[changed],
	))
	return changed, nil
}

func (s *directCodingSession) Delete(path string) (bool, error) {
	if err := rejectDirectCodingProtectedMutation(path, s.protectedPaths); err != nil {
		return false, err
	}
	target, err := resolveV3WorkspaceFile(s.root, path)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_file_delete_skipped", "path="+safeLine(path, "unknown")+" reason=missing")
			return false, nil
		}
		return false, fmt.Errorf("inspect coding delete target %s: %w", path, err)
	}
	result, err := s.executeWorkspaceMutation(workspaceFileMutation{
		Path: path, Operation: workspaceFileDelete,
	})
	if err != nil {
		return false, fmt.Errorf("delete coding file %s: %w", path, err)
	}
	s.emitReviewableDiff(path, result)
	s.recordDeletedSource(path)
	s.recordMutation(path, workspaceFileDelete)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_file_deleted", fmt.Sprintf(
		"path=%s result=%s",
		safeLine(path, "unknown"),
		safeLine(result.Summary, "deleted"),
	))
	return true, nil
}

func (s *directCodingSession) MaterializeTask(task directCodingFileTask) (bool, error) {
	if err := task.validate(); err != nil {
		return false, err
	}
	if s.cognition == nil {
		return false, fmt.Errorf("file materialization requires persisted task cognition")
	}
	if err := s.cognition.BeginTreeFile(task.Path); err != nil {
		return false, err
	}
	if err := rejectDirectCodingProtectedMutation(task.Path, s.protectedPaths); err != nil {
		return false, err
	}
	target, err := resolveV3WorkspaceFile(s.root, task.Path)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, fmt.Errorf("ensure coding target directory for %s: %w", task.Path, err)
	}
	operation := workspaceFileCreate
	current, readErr := os.ReadFile(target)
	if readErr == nil {
		operation = workspaceFileReplace
	} else if !os.IsNotExist(readErr) {
		return false, fmt.Errorf("inspect coding target %s: %w", task.Path, readErr)
	}
	if task.Content != "" {
		if err := s.validateProgramSource(task.Path, task.Content); err != nil {
			return false, err
		}
		changed, err := s.writeDirectCodingSource(task.Path, operation, current, task.Content)
		if err != nil {
			return false, err
		}
		if err := s.cognition.CompleteTreeFile(task.Path, "file="+task.Path+" sha256="+directCodingDigest(task.Content)); err != nil {
			return false, err
		}
		return changed, nil
	}
	return false, fmt.Errorf("code-owned source %s is empty", task.Path)
}

func (s *directCodingSession) writeDirectCodingSource(
	path string,
	operation workspaceFileOperation,
	current []byte,
	content string,
) (bool, error) {
	if bytes.Equal(current, []byte(content)) {
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_file_unchanged", fmt.Sprintf(
			"path=%s", safeLine(path, "unknown"),
		))
		return false, nil
	}
	result, writeErr := s.executeWorkspaceMutation(workspaceFileMutation{
		Path: path, Operation: operation, Content: content,
	})
	if writeErr != nil {
		return false, fmt.Errorf("write generated coding source %s: %w", path, writeErr)
	}
	s.emitReviewableDiff(path, result)
	s.recordWrittenSource(path, content)
	s.recordMutation(path, operation)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_file_written", fmt.Sprintf(
		"path=%s bytes=%d operation=%s result=%s",
		safeLine(path, "unknown"),
		len(content),
		operation,
		safeLine(result.Summary, "accepted"),
	))
	return true, nil
}

func (s *directCodingSession) emitReviewableDiff(path string, result operation.Result) {
	diff, _ := result.Output["diff"].(string)
	if diff == "" {
		return
	}
	s.runtime.svc.emitStepContextWithBudget(
		s.runtime.claim.Authority,
		"coding_diff",
		"path="+path+"\n"+diff,
		maxV3DiffPreviewBytes+512,
	)
}

func (s *directCodingSession) recordWrittenSource(path, content string) {
	sequence := s.nextSequence()
	s.completion.MutationCount++
	s.completion.LatestMutationTurn = sequence
	s.completion.WrittenSource[path] = content
}

func (s *directCodingSession) recordEnsuredDirectory(path string) {
	sequence := s.nextSequence()
	s.completion.MutationCount++
	s.completion.LatestMutationTurn = sequence
	s.recordMutation(path, workspaceDirectoryEnsure)
}

func (s *directCodingSession) recordDeletedSource(path string) {
	sequence := s.nextSequence()
	s.completion.MutationCount++
	s.completion.LatestMutationTurn = sequence
	delete(s.completion.WrittenSource, path)
}

func (s *directCodingSession) recordMutation(path string, operation workspaceFileOperation) {
	s.mutationJournal = append(s.mutationJournal, directCodingMutationJournalEntry{
		Path: path, Operation: operation,
	})
}

func (s *directCodingSession) executeWorkspaceMutation(command workspaceFileMutation) (operation.Result, error) {
	result, err := applyWorkspaceFileMutation(s.runtime.ctx, s.root, command)
	if err != nil {
		return operation.Result{}, err
	}
	if err := s.persistCodeOwnedEvidence(result); err != nil {
		return operation.Result{}, err
	}
	return result, nil
}

func (s *directCodingSession) persistCodeOwnedEvidence(result operation.Result) error {
	if len(result.Evidence) == 0 {
		return fmt.Errorf("code-owned operation produced no evidence")
	}
	for _, record := range result.Evidence {
		if err := s.runtime.writeEvidence(record); err != nil {
			return err
		}
	}
	return nil
}
