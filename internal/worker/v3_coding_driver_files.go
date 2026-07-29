package worker

import (
	"bytes"
	"fmt"
	"os"

	toolruntime "github.com/gryph/omnidex/internal/tools"
)

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
			s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_file_delete_skipped", "path="+safeLine(path, "unknown")+" reason=missing")
			return false, nil
		}
		return false, fmt.Errorf("inspect coding delete target %s: %w", path, err)
	}
	result, err := s.execute(toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path":      path,
		"operation": "delete",
		"content":   "",
	}})
	if err != nil {
		return false, fmt.Errorf("delete coding file %s: %w", path, err)
	}
	s.emitReviewableDiff(path, result)
	s.recordDeletedSource(path)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_file_deleted", fmt.Sprintf(
		"path=%s result=%s",
		safeLine(path, "unknown"),
		safeLine(result.Summary, "deleted"),
	))
	return true, nil
}

func (s *directCodingSession) Generate(task directCodingFileTask) (bool, error) {
	if err := task.validate(); err != nil {
		return false, err
	}
	if err := rejectDirectCodingProtectedMutation(task.Path, s.protectedPaths); err != nil {
		return false, err
	}
	target, err := resolveV3WorkspaceFile(s.root, task.Path)
	if err != nil {
		return false, err
	}
	operation := "create"
	current, readErr := os.ReadFile(target)
	if readErr == nil {
		operation = "replace"
	} else if !os.IsNotExist(readErr) {
		return false, fmt.Errorf("inspect coding target %s: %w", task.Path, readErr)
	}
	if task.Content != "" {
		if err := s.validateProgramSource(task.Path, task.Content); err != nil {
			return false, err
		}
		return s.writeDirectCodingSource(task.Path, operation, current, task.Content)
	}
	return false, fmt.Errorf("code-owned source %s is empty", task.Path)
}

func (s *directCodingSession) writeDirectCodingSource(
	path string,
	operation string,
	current []byte,
	content string,
) (bool, error) {
	if bytes.Equal(current, []byte(content)) {
		s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_file_unchanged", fmt.Sprintf(
			"path=%s", safeLine(path, "unknown"),
		))
		return false, nil
	}
	result, writeErr := s.execute(toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path":      path,
		"operation": operation,
		"content":   content,
	}})
	if writeErr != nil {
		return false, fmt.Errorf("write generated coding source %s: %w", path, writeErr)
	}
	s.emitReviewableDiff(path, result)
	s.recordWrittenSource(path, content)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_file_written", fmt.Sprintf(
		"path=%s bytes=%d operation=%s result=%s",
		safeLine(path, "unknown"),
		len(content),
		operation,
		safeLine(result.Summary, "accepted"),
	))
	return true, nil
}

func (s *directCodingSession) emitReviewableDiff(path string, result toolruntime.Result) {
	diff, _ := result.Output["diff"].(string)
	if diff == "" {
		return
	}
	s.runtime.svc.emitStepContextWithBudget(
		s.runtime.claim.Step.ID,
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

func (s *directCodingSession) recordDeletedSource(path string) {
	sequence := s.nextSequence()
	s.completion.MutationCount++
	s.completion.LatestMutationTurn = sequence
	delete(s.completion.WrittenSource, path)
}

func (s *directCodingSession) execute(call toolruntime.Call) (toolruntime.Result, error) {
	return s.runtime.svc.executeV3Tool(s.runtime.ctx, s.runtime.claim, "subtask_executor", call)
}
