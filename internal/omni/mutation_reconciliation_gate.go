package omni

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

type mutationMoveSpec struct {
	OldRel string
	NewRel string
	OldAbs string
	NewAbs string
}

func mutationReconciliationMoveSpec(command, workingDirectory string) (mutationMoveSpec, bool) {
	segments := structuredCommandSegments(command)
	if len(segments) == 0 {
		return mutationMoveSpec{}, false
	}
	for _, segment := range segments {
		if len(segment) == 0 || cleanCommandPathToken(segment[0]) != "mv" {
			continue
		}
		args := []string{}
		stopOptions := false
		for _, raw := range segment[1:] {
			arg := cleanCommandPathToken(raw)
			if arg == "" {
				continue
			}
			if arg == "--" {
				stopOptions = true
				continue
			}
			if !stopOptions && strings.HasPrefix(arg, "-") {
				continue
			}
			args = append(args, arg)
		}
		if len(args) != 2 {
			return mutationMoveSpec{}, false
		}
		oldRel := filepath.ToSlash(filepath.Clean(args[0]))
		newRel := filepath.ToSlash(filepath.Clean(args[1]))
		if oldRel == "." || newRel == "." || strings.TrimSpace(workingDirectory) == "" {
			return mutationMoveSpec{}, false
		}
		return mutationMoveSpec{
			OldRel: oldRel,
			NewRel: newRel,
			OldAbs: mutationGateAbsPath(workingDirectory, oldRel),
			NewAbs: mutationGateAbsPath(workingDirectory, newRel),
		}, true
	}
	return mutationMoveSpec{}, false
}

func mutationGateAbsPath(workingDirectory, rel string) string {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Join(workingDirectory, filepath.FromSlash(rel))
}

func applyMutationReconciliationGateBeforeMove(step int, command, commandID, workingDirectory string, spec mutationMoveSpec, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	oldExists := fileExists(spec.OldAbs)
	newExists := fileExists(spec.NewAbs)
	switch {
	case oldExists && !newExists:
		return false, nil
	case !oldExists && newExists:
		stdoutText := fmt.Sprintf("file_move_already_reconciled %s -> %s\n", spec.OldRel, spec.NewRel)
		emitStructuredCommandEvent(onEvent, "file_move_already_reconciled", "File move already reconciled before command execution", map[string]string{
			"step":     fmt.Sprintf("%d", step),
			"command":  truncateStructuredTimelineValue(command),
			"old_path": spec.OldRel,
			"new_path": spec.NewRel,
		})
		stdoutText += refreshWorkspaceRouteAfterMutation(workingDirectory, spec, onEvent)
		appendMutationGateObservation(step, command, commandID, workingDirectory, 0, stdoutText, "", result)
		markMatchingMoveObjectivesComplete(spec, result)
		return true, nil
	case oldExists && newExists:
		stderrText := fmt.Sprintf("mutation_reconciliation_gate: move conflict: both %s and %s exist", spec.OldRel, spec.NewRel)
		appendMutationGateObservation(step, command, commandID, workingDirectory, 1, "", stderrText, result)
		return true, nil
	default:
		stderrText := fmt.Sprintf("mutation_reconciliation_gate: move cannot run: %s absent and %s absent; no such file or directory: %s", spec.OldRel, spec.NewRel, spec.OldRel)
		appendMutationGateObservation(step, command, commandID, workingDirectory, 1, "", stderrText, result)
		return true, nil
	}
}

func verifyMutationReconciliationGateAfterMove(step int, command, workingDirectory string, spec mutationMoveSpec, stdoutBuf, stderrBuf *bytes.Buffer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	if !fileExists(spec.NewAbs) || fileExists(spec.OldAbs) {
		if stderrBuf.Len() > 0 {
			stderrBuf.WriteString("\n")
		}
		stderrBuf.WriteString(fmt.Sprintf("mutation_reconciliation_gate: move verification failed: new_exists=%t old_absent=%t", fileExists(spec.NewAbs), !fileExists(spec.OldAbs)))
		return false
	}
	if stdoutBuf.Len() > 0 && !strings.HasSuffix(stdoutBuf.String(), "\n") {
		stdoutBuf.WriteString("\n")
	}
	stdoutBuf.WriteString(fmt.Sprintf("file_move_verified %s -> %s\n", spec.OldRel, spec.NewRel))
	emitStructuredCommandEvent(onEvent, "file_move_verified", "File move verified after mutation", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"command":  truncateStructuredTimelineValue(command),
		"old_path": spec.OldRel,
		"new_path": spec.NewRel,
	})
	stdoutBuf.WriteString(refreshWorkspaceRouteAfterMutation(workingDirectory, spec, onEvent))
	markMatchingMoveObjectivesComplete(spec, result)
	return true
}

func appendMutationGateObservation(step int, command, commandID, workingDirectory string, exitCode int, stdoutText, stderrText string, result *CommandDecisionResult) {
	if result == nil {
		return
	}
	result.Command = command
	result.ExitCode = exitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:      step,
		CommandID: commandID,
		Command:   command,
		ExitCode:  exitCode,
		Stdout:    truncateStructuredObservation(stdoutText),
		Stderr:    truncateStructuredObservation(stderrText),
		CWD:       structuredPromptWorkingDirectory(workingDirectory),
	})
}

func refreshWorkspaceRouteAfterMutation(workingDirectory string, spec mutationMoveSpec, onEvent func(StructuredCommandEvent)) string {
	if strings.TrimSpace(workingDirectory) == "" {
		return ""
	}
	indexPath := filepath.Join(workingDirectory, ".omni", "index.json")
	index, err := UpdateWorkspaceIndex(workingDirectory, indexPath, 0)
	if err != nil {
		return ""
	}
	if err := WriteWorkspaceIndex(index, indexPath); err != nil {
		return ""
	}
	cm, err := UpdateCodebaseMap(workingDirectory, DefaultCodebaseMapPath(workingDirectory), CodebaseMapConfig{})
	if err == nil {
		if err := WriteCodebaseMap(cm, DefaultCodebaseMapPath(workingDirectory)); err != nil {
			return ""
		}
	}
	emitStructuredCommandEvent(onEvent, "workspace_route_refreshed_after_mutation", "Workspace index and codebase route refreshed after mutation", map[string]string{
		"old_path": spec.OldRel,
		"new_path": spec.NewRel,
	})
	return "workspace_route_refreshed_after_mutation\n"
}

func markMatchingMoveObjectivesComplete(spec mutationMoveSpec, result *CommandDecisionResult) {
	if result == nil || len(result.ObjectiveLedger) == 0 {
		return
	}
	for i := range result.ObjectiveLedger {
		objective := result.ObjectiveLedger[i]
		if structuredObjectiveSatisfied(objective) || !moveObjectiveMatchesSpec(objective, spec) {
			continue
		}
		objective.Status = "satisfied"
		objective.Evidence = fmt.Sprintf("mutation_reconciliation_gate:%s->%s", spec.OldRel, spec.NewRel)
		result.ObjectiveLedger[i] = objective
	}
}

func moveObjectiveMatchesSpec(objective StructuredObjective, spec mutationMoveSpec) bool {
	hasOldAbsent := false
	hasNewExists := false
	for _, predicate := range objective.RequiredEvidence {
		kind, rest, ok := strings.Cut(predicate, ":")
		if !ok {
			continue
		}
		path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rest)))
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "file_absent":
			hasOldAbsent = hasOldAbsent || path == spec.OldRel
		case "file_exists", "file_nonempty":
			hasNewExists = hasNewExists || path == spec.NewRel
		}
	}
	if hasOldAbsent && hasNewExists {
		return true
	}
	target := strings.ToLower(objective.ID + " " + objective.Description + " " + strings.Join(objective.RequiredEvidence, " "))
	return strings.Contains(target, strings.ToLower(spec.OldRel)) &&
		strings.Contains(target, strings.ToLower(spec.NewRel)) &&
		(strings.Contains(target, "rename") || strings.Contains(target, "move"))
}

func removeRouteFile(route TaskRoute, path string) TaskRoute {
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	next := route
	out := []string{}
	for _, file := range route.LikelyFiles {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(file)))
		if clean == "." || clean == path {
			continue
		}
		out = append(out, file)
	}
	next.LikelyFiles = out
	return next
}

func addRouteFile(route TaskRoute, path string) TaskRoute {
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if path == "." {
		return route
	}
	next := route
	for _, file := range next.LikelyFiles {
		if filepath.ToSlash(filepath.Clean(strings.TrimSpace(file))) == path {
			return next
		}
	}
	next.LikelyFiles = append(next.LikelyFiles, path)
	return next
}

func taskRouteAfterMutationMove(route TaskRoute, oldPath, newPath string) TaskRoute {
	return addRouteFile(removeRouteFile(route, oldPath), newPath)
}
