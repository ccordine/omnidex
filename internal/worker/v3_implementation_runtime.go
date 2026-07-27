package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const (
	subtaskExecutionModeGeneral        = "general_tool_loop"
	subtaskExecutionModeImplementation = "implementation_ledger"
)

func requiresImplementationLedger(objective artifacts.Objective) bool {
	return objective.RequiresAction &&
		containsString(objective.RequiredCapabilities, capabilityWorkspaceWrite) &&
		containsString(objective.RequiredCapabilities, capabilityCommandExecute)
}

func executionModeForObjective(objective artifacts.Objective) string {
	if requiresImplementationLedger(objective) {
		return subtaskExecutionModeImplementation
	}
	return subtaskExecutionModeGeneral
}

func (r *nativeRuntimeV3) runImplementationObjective(assignment v3SubtaskAssignment, objective artifacts.Objective) (string, []string, error) {
	if r == nil || r.svc == nil || r.svc.v3Tools == nil {
		return "", nil, fmt.Errorf("implementation runtime unavailable")
	}
	if !requiresImplementationLedger(objective) {
		return "", nil, fmt.Errorf("objective %q does not authorize implementation ledger execution", objective.ID)
	}
	spec, ok := r.svc.skillSpec(assignment.RoleID)
	if !ok {
		return "", nil, fmt.Errorf("assigned specialist %q is not registered", assignment.RoleID)
	}
	available := toolSpecNames(r.availableToolSpecs(assignment.RoleID, assignment.RequiredCapabilities))
	for _, required := range []string{"workspace.write", "command.run"} {
		if !containsString(available, required) {
			return "", nil, fmt.Errorf("implementation runtime requires available tool %q", required)
		}
	}
	criteria := cleanOrderedStrings(append(append([]string(nil), assignment.SuccessCriteria...), objective.AcceptanceCriteria...))
	constraints := cleanOrderedStrings(assignment.Constraints)
	ledger, found, err := r.loadImplementationLedger(objective, constraints, criteria)
	if err != nil {
		return "", nil, err
	}
	if !found {
		ledger, err = r.createImplementationLedger(objective, constraints, criteria)
		if err != nil {
			return "", nil, err
		}
	}

	records := make([]subtaskToolRecord, 0, len(ledger.Items)+maxImplementationVerifyRuns)
	for {
		index, err := readyImplementationWorkItem(ledger)
		if err != nil {
			return "", nil, err
		}
		if index < 0 {
			scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
			if err != nil {
				return "", nil, err
			}
			if err := validateCompletedImplementationLedger(scope.Root, ledger); err != nil {
				return "", nil, err
			}
			summary := implementationLedgerSummary(ledger)
			if err := spec.ValidateOutputPayload(map[string]any{
				"summary": summary, "sources": []string{"workspace"}, "tool_calls": flattenToolRecords(records),
			}); err != nil {
				return "", nil, err
			}
			r.svc.emitStepEvent(r.claim.Step.ID, "implementation_objective_completed", fmt.Sprintf("objective=%s revision=%d items=%d", objective.ID, ledger.Revision, len(ledger.Items)))
			return summary, []string{"workspace"}, nil
		}

		item := ledger.Items[index]
		var record *subtaskToolRecord
		switch item.Kind {
		case artifacts.ImplementationWorkKindFile:
			record, err = r.runImplementationFileItem(&ledger, index, spec)
		case artifacts.ImplementationWorkKindVerification:
			record, err = r.runImplementationVerificationItem(&ledger, index, spec)
		default:
			err = fmt.Errorf("implementation work item %q has unsupported kind %q", item.ID, item.Kind)
		}
		if record != nil {
			records = append(records, *record)
		}
		if err != nil {
			return "", nil, err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_ledger_progress", implementationProgressSummary(ledger))
	}
}

func validateCompletedImplementationLedger(root string, ledger artifacts.ImplementationLedgerArtifact) error {
	if err := validateImplementationLedger(ledger); err != nil {
		return err
	}
	for _, item := range ledger.Items {
		if item.Status != artifacts.ImplementationWorkStatusCompleted {
			return fmt.Errorf("implementation work item %q is %s at completion", item.ID, item.Status)
		}
		switch item.Kind {
		case artifacts.ImplementationWorkKindFile:
			if strings.TrimSpace(item.ContentSHA256) == "" {
				return fmt.Errorf("completed file work item %q has no content hash", item.ID)
			}
			target, err := resolveV3WorkspaceFile(root, item.Path)
			if err != nil {
				return err
			}
			content, err := os.ReadFile(target)
			if err != nil {
				return fmt.Errorf("read completed implementation file %s: %w", item.Path, err)
			}
			digest := sha256.Sum256(content)
			if hex.EncodeToString(digest[:]) != item.ContentSHA256 {
				return fmt.Errorf("completed file %q changed outside its one-writer work item", item.Path)
			}
		case artifacts.ImplementationWorkKindVerification:
			if item.Attempts < 1 || strings.TrimSpace(item.ResultSummary) == "" {
				return fmt.Errorf("completed verification item %q has no successful command evidence", item.ID)
			}
		}
	}
	return nil
}

func implementationProgressSummary(ledger artifacts.ImplementationLedgerArtifact) string {
	counts := map[string]int{}
	active := "none"
	for _, item := range ledger.Items {
		counts[item.Status]++
		if item.Status == artifacts.ImplementationWorkStatusRunning {
			active = item.ID
		}
	}
	return fmt.Sprintf("revision=%d completed=%d pending=%d running=%d failed=%d active=%s",
		ledger.Revision,
		counts[artifacts.ImplementationWorkStatusCompleted],
		counts[artifacts.ImplementationWorkStatusPending],
		counts[artifacts.ImplementationWorkStatusRunning],
		counts[artifacts.ImplementationWorkStatusFailed],
		active,
	)
}
