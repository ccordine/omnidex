package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func (r *nativeRuntimeV3) loadImplementationLedger(objective artifacts.Objective, constraints, criteria []string) (artifacts.ImplementationLedgerArtifact, bool, error) {
	envelope, found, err := r.svc.repo.LatestArtifact(r.ctx, r.claim.Job.ID, artifacts.KindImplementationLedger)
	if err != nil {
		return artifacts.ImplementationLedgerArtifact{}, false, fmt.Errorf("read implementation ledger: %w", err)
	}
	if !found {
		return artifacts.ImplementationLedgerArtifact{}, false, nil
	}
	ledger, err := requireArtifactPayload[artifacts.ImplementationLedgerArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindImplementationLedger)
	if err != nil {
		return artifacts.ImplementationLedgerArtifact{}, false, err
	}
	if ledger.ObjectiveID != strings.TrimSpace(objective.ID) || ledger.Objective != strings.TrimSpace(objective.Description) {
		return artifacts.ImplementationLedgerArtifact{}, false, fmt.Errorf("implementation ledger authority mismatch for objective %q", objective.ID)
	}
	if strings.Join(ledger.Constraints, "\x00") != strings.Join(cleanOrderedStrings(constraints), "\x00") {
		return artifacts.ImplementationLedgerArtifact{}, false, fmt.Errorf("implementation ledger constraints changed after planning")
	}
	if strings.Join(ledger.AcceptanceCriteria, "\x00") != strings.Join(cleanOrderedStrings(criteria), "\x00") {
		return artifacts.ImplementationLedgerArtifact{}, false, fmt.Errorf("implementation ledger acceptance criteria changed after planning")
	}
	if err := validateImplementationLedger(ledger); err != nil {
		return artifacts.ImplementationLedgerArtifact{}, false, err
	}
	interrupted := false
	for index := range ledger.Items {
		item := &ledger.Items[index]
		if item.Status != artifacts.ImplementationWorkStatusRunning {
			continue
		}
		item.Status = artifacts.ImplementationWorkStatusPending
		item.LastError = "The previous worker stopped while this item was running. Reconcile the current target file and continue this same contract."
		interrupted = true
	}
	if interrupted {
		ledger.Revision++
		if err := r.persistImplementationLedger(ledger); err != nil {
			return artifacts.ImplementationLedgerArtifact{}, false, err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_ledger_recovered", fmt.Sprintf("revision=%d action=reopen_interrupted_items", ledger.Revision))
	}
	_ = envelope
	return ledger, true, nil
}

func (r *nativeRuntimeV3) persistImplementationLedger(ledger artifacts.ImplementationLedgerArtifact) error {
	if err := validateImplementationLedger(ledger); err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindImplementationLedger, ledger); err != nil {
		return fmt.Errorf("persist implementation ledger revision %d: %w", ledger.Revision, err)
	}
	return nil
}

func implementationLedgerSummary(ledger artifacts.ImplementationLedgerArtifact) string {
	files := make([]string, 0, len(ledger.Items))
	verification := ""
	for _, item := range ledger.Items {
		switch item.Kind {
		case artifacts.ImplementationWorkKindFile:
			files = append(files, item.Path)
		case artifacts.ImplementationWorkKindVerification:
			verification = strings.TrimSpace(item.ResultSummary)
		}
	}
	return strings.Join([]string{
		fmt.Sprintf("Completed implementation objective %s through %d bounded work items.", ledger.ObjectiveID, len(ledger.Items)),
		"Files: " + strings.Join(files, ", "),
		"Verification: " + safeLine(verification, "completed"),
	}, " ")
}
