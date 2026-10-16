package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// auditAndFreezeCodingLedger is the production sign-off boundary. It reloads
// every authority from PostgreSQL, persists the first audit call, resumes from
// that receipt after interruption, and only then freezes that exact revision.
func (s *Server) auditAndFreezeCodingLedger(ctx context.Context, jobID int64, request codingPlanFreezeRequest) (queue.CodingLedgerRevision, error) {
	current, found, err := s.repo.CurrentCodingLedgerForJob(ctx, jobID)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("coding ledger not found")
		}
		return queue.CodingLedgerRevision{}, err
	}
	auditID := "planning-audit:" + request.OperationID
	if _, replay, lookupErr := s.repo.CodingLedgerRevisionForOperation(ctx, current.LedgerID, request.OperationID); lookupErr != nil {
		return queue.CodingLedgerRevision{}, lookupErr
	} else if replay {
		frozen, freezeErr := s.repo.FreezeCodingLedger(ctx, queue.FreezeCodingLedgerCommand{
			LedgerID: current.LedgerID, Revision: request.Revision, OperationID: request.OperationID,
			OwnerTurnID: request.OwnerTurnID, OwnerTurnText: request.OwnerTurnText, AuditID: auditID,
		})
		if freezeErr != nil {
			return queue.CodingLedgerRevision{}, freezeErr
		}
		if err := s.repo.CompleteCodingLedgerReview(ctx, jobID, frozen.LedgerID, frozen.Revision); err != nil {
			return queue.CodingLedgerRevision{}, err
		}
		return frozen, nil
	}
	if current.Revision != request.Revision {
		return queue.CodingLedgerRevision{}, fmt.Errorf("%w: displayed revision %d is stale; current revision is %d", queue.ErrCodingLedgerRevision, request.Revision, current.Revision)
	}
	turn := codingagent.PlanningTurn{OperationID: request.OperationID, OwnerTurnID: request.OwnerTurnID,
		Text: request.OwnerTurnText, ObservedRevision: request.Revision}
	persisted, found, err := s.repo.CodingLedgerPlanningAudit(ctx, current.LedgerID, auditID)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	var audit codingagent.PlanningAuditReceipt
	if found {
		if persisted.Revision != current.Revision {
			return queue.CodingLedgerRevision{}, fmt.Errorf("%w: persisted planning audit revision %d does not match displayed revision %d", queue.ErrCodingLedgerRevision, persisted.Revision, current.Revision)
		}
		audit = persistedPlanningAuditReceipt(persisted)
	} else {
		controller, controllerErr := s.codingLedgerPlanningController(ctx, jobID, request.WorkspaceRoot.Value)
		if controllerErr != nil {
			return queue.CodingLedgerRevision{}, controllerErr
		}
		turns, turnsErr := s.repo.CodingLedgerPlanningTurns(ctx, current.LedgerID)
		if turnsErr != nil {
			return queue.CodingLedgerRevision{}, turnsErr
		}
		audit, err = controller.AuditPlanningLedger(ctx, auditID, turn, current.Ledger, turns)
		if err != nil {
			return queue.CodingLedgerRevision{}, err
		}
		if err := s.repo.RecordCodingLedgerPlanningAudit(ctx, queue.RecordCodingLedgerPlanningAuditCommand{
			LedgerID: current.LedgerID, AuditID: audit.AuditID, Revision: audit.Revision,
			Ready: audit.Ready, Explanation: audit.Judgment.Explanation,
		}); err != nil {
			return queue.CodingLedgerRevision{}, err
		}
	}
	if !audit.Ready {
		return queue.CodingLedgerRevision{}, fmt.Errorf("%w: planning audit requires owner direction: %s", queue.ErrCodingLedgerState, audit.Judgment.Explanation)
	}
	if err := codingagent.ValidatePlanningSignoff(current.Ledger, audit); err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	frozen, err := s.repo.FreezeCodingLedger(ctx, queue.FreezeCodingLedgerCommand{
		LedgerID: current.LedgerID, Revision: request.Revision, OperationID: request.OperationID,
		OwnerTurnID: request.OwnerTurnID, OwnerTurnText: request.OwnerTurnText, AuditID: audit.AuditID,
	})
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	if err := s.repo.CompleteCodingLedgerReview(ctx, jobID, frozen.LedgerID, frozen.Revision); err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	return frozen, nil
}

func (s *Server) codingLedgerPlanningController(ctx context.Context, jobID int64, root string) (*codingagent.Controller, error) {
	details, err := s.repo.CurrentJobDetails(ctx, jobID)
	if err != nil {
		return nil, err
	}
	configuration, err := codingagent.ConfigurationFromMetadata(details.Job.Metadata)
	if err != nil {
		return nil, err
	}
	if configuration.Mode != codingagent.Agents {
		return nil, fmt.Errorf("normalized ledger planning requires Agents mode")
	}
	timeout, err := time.ParseDuration(s.providerConfig.RequestTimeout)
	if err != nil {
		return nil, err
	}
	provider, err := codex.New(s.providerConfig.CodexCommand, s.providerConfig.CodexDirectory, timeout)
	if err != nil {
		return nil, err
	}
	return &codingagent.Controller{
		JobID: jobID, Generation: details.Job.CurrentGeneration, Root: root, Configuration: configuration,
		Requirements: []codingagent.Requirement{{ID: "objective", Outcome: details.Job.Instruction}},
		Objective:    &codingagent.Objective{Instruction: details.Job.Instruction},
		Providers:    map[string]codingagent.Provider{configuration.Planner.Provider: provider},
		Environment:  planningAuditEnvironment{}, Evidence: &planningAuditEvidence{},
	}, nil
}

func (s *Server) analyzeDirectPlanningCoverage(ctx context.Context, jobID int64, root string, turn codingagent.PlanningTurn, current queue.CodingLedgerRevision) (queue.CodingLedgerRevision, error) {
	controller, err := s.codingLedgerPlanningController(ctx, jobID, root)
	if err != nil {
		return current, err
	}
	turn.ObservedRevision = current.Revision
	judgment, err := controller.AnalyzePlanningCoverage(ctx, turn, current.Ledger,
		model.OwnerTurnDisposition{Statements: current.OwnerTurnDispositions})
	if err != nil {
		return current, err
	}
	if judgment.Covered {
		return current, nil
	}
	return current, uncoveredPlanningCoverageError(judgment)
}

func persistedPlanningAuditReceipt(persisted queue.CodingLedgerPlanningAudit) codingagent.PlanningAuditReceipt {
	return codingagent.PlanningAuditReceipt{
		AuditID: persisted.AuditID, Revision: persisted.Revision, Ready: persisted.Ready,
		Judgment: codingagent.PlanningCoverageJudgment{
			Question: codingagent.AuditPlanningQuestion, BaseRevision: persisted.Revision,
			Covered: persisted.Ready, Explanation: persisted.Explanation,
		},
	}
}

func uncoveredPlanningCoverageError(judgment codingagent.PlanningCoverageJudgment) error {
	details := judgment.Explanation
	if len(judgment.Suggestions) > 0 {
		details += "; suggestions: " + strings.Join(judgment.Suggestions, "; ")
	}
	return fmt.Errorf("%w: planning coverage requires owner direction: %s", queue.ErrCodingLedgerState, details)
}

type planningAuditEnvironment struct{}

func (planningAuditEnvironment) ReadFiles(context.Context) ([]string, error) {
	return nil, fmt.Errorf("planning audit cannot enumerate files")
}
func (planningAuditEnvironment) ReadTaskFile(context.Context, string) (string, error) {
	return "", fmt.Errorf("planning audit cannot read task files")
}
func (planningAuditEnvironment) PrepareFiles(context.Context, codingagent.Task) error {
	return fmt.Errorf("planning audit cannot prepare files")
}
func (planningAuditEnvironment) Attest(context.Context) error { return nil }
func (planningAuditEnvironment) WriteFile(context.Context, codingagent.FileWrite) error {
	return fmt.Errorf("planning audit is read-only")
}
func (planningAuditEnvironment) DeleteFile(context.Context, codingagent.FileDelete) error {
	return fmt.Errorf("planning audit is read-only")
}

type planningAuditEvidence struct {
	mu      sync.Mutex
	entries []codingagent.Entry
}

func (store *planningAuditEvidence) Append(_ context.Context, entry codingagent.Entry) (int64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry.ID = int64(len(store.entries) + 1)
	store.entries = append(store.entries, entry)
	return entry.ID, nil
}
func (store *planningAuditEvidence) Read(context.Context) ([]codingagent.Entry, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]codingagent.Entry(nil), store.entries...), nil
}
