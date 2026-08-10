package cognitionruntime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

type runtimeFixture struct {
	binding     Binding
	replacement Binding
	goal        cognition.GoalExpression
	schema      cognition.ActionSchema
	catalog     cognition.ActionCatalog
	evidence    cognition.EvidenceRef
	graph       cognition.ObligationGraphSnapshot
	revision    cognition.WorldRevision
}

func newRuntimeFixture(t *testing.T) runtimeFixture {
	t.Helper()
	episode, err := cognition.NewEpisodeRef("episode-runtime-1")
	requireNoError(t, err)
	attempt := cognition.AttemptRef{JobID: 41, Generation: 3, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	replacement := attempt
	replacement.Attempt, replacement.WorkerID = 2, "worker-b"
	revision, err := cognition.NewWorldRevision(episode.ID, 1, runtimeDigest("revision-1"))
	requireNoError(t, err)
	observation, err := cognition.NewObservation("observation-initial", revision, "state", "Required evidence.")
	requireNoError(t, err)
	predicate, err := cognition.NewPredicate("objective.ready", []string{"target"})
	requireNoError(t, err)
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	requireNoError(t, err)
	check := cognition.CompletionCheckRef{
		ID: "completion-check", Version: "1.0.0", SHA256: runtimeDigest("completion-check"),
	}
	root := cognition.ObligationSpec{
		ID: "obligation-root", Desired: goal, SupportingRefs: []cognition.EvidenceRef{observation.EvidenceRef()},
		CompletionCheck: check,
	}
	graph, err := cognition.NewObligationGraph(cognition.InitialObligationGeneration, root.ID, []cognition.ObligationSpec{root})
	requireNoError(t, err)
	requireNoError(t, graph.RefreshReadiness(cognition.InitialObligationGeneration))
	requireNoError(t, graph.Transition(root.ID, cognition.InitialObligationGeneration, cognition.ObligationActive))
	schema, err := cognition.NewActionSchema(
		"action-schema", "1.0.0", "operate",
		[]cognition.ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 64}}, cognition.EvidenceRequired,
	)
	requireNoError(t, err)
	catalog, err := cognition.NewActionCatalog("action-catalog", "1.0.0", []cognition.ActionSchema{schema})
	requireNoError(t, err)
	return runtimeFixture{
		binding:     Binding{Episode: episode, Attempt: attempt},
		replacement: Binding{Episode: episode, Attempt: replacement}, goal: goal,
		schema: schema, catalog: catalog, evidence: observation.EvidenceRef(), graph: graph.Snapshot(), revision: revision,
	}
}

type runtimeHarness struct {
	fixture  runtimeFixture
	graph    cognition.ObligationGraphSnapshot
	version  uint64
	journal  cognition.WorldRevision
	env      cognition.WorldRevision
	terminal bool
	public   string

	policyCalls, completionCalls, environmentCalls  int
	policyProviderRequestDispatched                 bool
	forceSatisfied, forceUnsatisfied, nextTerminal  bool
	useModelEvidenceOverride                        bool
	modelEvidenceOverride, completionResultEvidence []cognition.EvidenceRef
	typedFailureCode                                cognition.ActionFailureCode
	remainingCalls                                  uint32
	corruptReceipt                                  bool
	corruptResolvedAction                           bool
	policyError, environmentError                   error
	dispatchCommitError                             bool
	transitionWriteFailures                         int
	sealFailures                                    int
	unresolved                                      *ActionRecord
	terminalProgress                                *EpisodeProgress
	acceptedRecovery                                *AcceptedDecisionRecovery
	terminalPolicyRecovered                         bool
	terminalPolicyError                             error
	abandonment                                     *PolicyCallAbandonment
	receipts                                        map[cognition.ActionID]cognition.Transition
	applied                                         []cognition.RegisteredAction
	order                                           []string
}

func newRuntimeHarness(t *testing.T) *runtimeHarness {
	t.Helper()
	fixture := newRuntimeFixture(t)
	return &runtimeHarness{
		fixture: fixture, graph: fixture.graph.Clone(), version: 1,
		journal: fixture.revision, env: fixture.revision,
		receipts: make(map[cognition.ActionID]cognition.Transition), remainingCalls: 8,
		policyProviderRequestDispatched: true,
	}
}

func (h *runtimeHarness) dependencies() Dependencies {
	return Dependencies{
		Policy: h, Environment: h, Snapshots: h, Accepted: h, PolicyRecovery: h, Completion: h,
		Episodes: h, Reconciler: h, Actions: h, TerminalSeal: h,
	}
}

func (h *runtimeHarness) PrepareSnapshot(_ context.Context, binding Binding) (PreparedSnapshot, error) {
	h.order = append(h.order, "snapshot")
	var active cognition.Obligation
	for _, obligation := range h.graph.Obligations {
		if obligation.Status == cognition.ObligationActive {
			active = obligation
		}
	}
	projection := cognition.ContextProjectionRef{
		ID:           cognition.ContextProjectionID(fmt.Sprintf("projection-%d", h.journal.Number)),
		SHA256:       runtimeDigest(fmt.Sprintf("projection-%d", h.journal.Number)),
		WorkingSetID: "working-set-runtime", WorkingSetVersion: h.journal.Number,
		RendererVersion: "1.0.0",
	}
	modelEvidence := []cognition.EvidenceRef{h.fixture.evidence}
	if h.useModelEvidenceOverride {
		modelEvidence = append([]cognition.EvidenceRef{}, h.modelEvidenceOverride...)
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		h.fixture.goal, h.journal, active, h.fixture.catalog, binding.Attempt, projection,
		cognition.RuntimeBudget{
			RemainingPolicyCalls: h.remainingCalls,
			MaxInputBytes:        64 * 1024,
			MaxInputTokens:       16 * 1024,
			MaxOutputBytes:       16 * 1024,
			MaxOutputTokens:      4 * 1024,
			MaxEvidenceRefs:      8,
			MaxActionArguments:   8,
			MaxLedgerProposals:   8, MaxAttentionRequests: 8, MaxExpectedEffectBytes: 256,
		},
		modelEvidence,
	)
	if err != nil {
		return PreparedSnapshot{}, err
	}
	return PreparedSnapshot{
		Snapshot: snapshot, ObligationGraph: h.graph.Clone(), GraphVersion: h.version,
		CompletionEvidenceRefs: []cognition.EvidenceRef{h.fixture.evidence},
		EnvironmentTerminal:    h.terminal, PublicOutcome: h.public,
	}, nil
}

func (h *runtimeHarness) Evaluate(_ context.Context, request CompletionRequest) (cognition.CompletionResult, error) {
	h.order = append(h.order, "completion")
	h.completionCalls++
	outcome := cognition.CompletionUnsatisfied
	var evidence []cognition.EvidenceRef
	if !h.forceUnsatisfied && (h.forceSatisfied || request.EnvironmentTerminal) {
		outcome, evidence = cognition.CompletionSatisfied, request.Obligation.SupportingRefs
		if h.completionResultEvidence != nil {
			evidence = append([]cognition.EvidenceRef{}, h.completionResultEvidence...)
		}
	}
	return cognition.NewCompletionResult(
		request.Obligation.ID, request.Obligation.CompletionCheck, request.Revision, outcome, evidence,
	)
}

func (h *runtimeHarness) Decide(_ context.Context, snapshot cognition.RuntimeSnapshot) (cognition.PolicyOutcome, error) {
	h.order = append(h.order, "policy")
	h.policyCalls++
	if h.policyError != nil {
		return cognition.PolicyOutcome{ProviderRequestDispatched: h.policyProviderRequestDispatched}, h.policyError
	}
	request, err := cognition.NewActionRequest("operate", []cognition.ActionArgument{{Name: "target", Value: "unit"}})
	if err != nil {
		return cognition.PolicyOutcome{ProviderRequestDispatched: h.policyProviderRequestDispatched}, err
	}
	return cognition.PolicyOutcome{Decision: cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID, Action: request,
		EvidenceRefs: []cognition.EvidenceRef{h.fixture.evidence}, ExpectedEffect: "The target changes state.",
	}, ProviderRequestDispatched: h.policyProviderRequestDispatched}, nil
}

func (h *runtimeHarness) Reconcile(
	_ context.Context,
	command ReconciliationCommand,
) (ReconciliationReceipt, error) {
	h.order = append(h.order, "reconcile")
	receipt, err := NewReconciliationReceipt(command, h.version, command.Projection.WorkingSetVersion)
	if h.corruptReceipt {
		receipt.DecisionSHA256 = runtimeDigest("changed-decision")
	}
	return receipt, err
}

func (h *runtimeHarness) Unresolved(_ context.Context, _ Binding) (*ActionRecord, error) {
	h.order = append(h.order, "unresolved")
	if h.unresolved == nil {
		return nil, nil
	}
	copy := h.unresolved.Clone()
	return &copy, nil
}

func (h *runtimeHarness) TerminalProgress(
	_ context.Context,
	_ Binding,
) (*EpisodeProgress, error) {
	h.order = append(h.order, "terminal-progress")
	if h.terminalProgress == nil {
		return nil, nil
	}
	copy := cloneEpisodeProgress(*h.terminalProgress)
	return &copy, nil
}

func (h *runtimeHarness) prepareAction(command PrepareActionCommand) (ActionRecord, error) {
	h.order = append(h.order, "prepare-action")
	if command.Coordinator.Decision == nil {
		return ActionRecord{}, errors.New("missing decision")
	}
	reconcile := ReconciliationCommand{
		Binding: command.Binding, SnapshotSHA256: command.Coordinator.SnapshotSHA256,
		Projection: command.Coordinator.ContextProjection, ActionSchema: h.fixture.schema,
		Decision: command.Coordinator.Decision.Clone(), Recovery: cloneRecoveryRef(command.Recovery),
	}
	if command.Recovery != nil && command.Reconciliation.Recovery == nil && h.acceptedRecovery != nil {
		if h.acceptedRecovery.ExistingReconciliation == nil {
			return ActionRecord{}, errors.New("missing existing reconciliation")
		}
		if err := command.Reconciliation.ValidateFor(h.acceptedRecovery.ExistingReconciliation.Command); err != nil {
			return ActionRecord{}, err
		}
	} else {
		if err := command.Reconciliation.ValidateFor(reconcile); err != nil {
			return ActionRecord{}, err
		}
	}
	decision := command.Coordinator.Decision.Clone()
	action, err := cognition.NewRegisteredAction(
		"action-runtime-1", command.Binding.Attempt, h.fixture.schema, decision.Action, decision.EvidenceRefs,
	)
	if err != nil {
		return ActionRecord{}, err
	}
	record := ActionRecord{
		Episode: command.Binding.Episode, ExpectedRevision: h.journal,
		SnapshotSHA256:    command.Coordinator.SnapshotSHA256,
		ContextProjection: command.Coordinator.ContextProjection,
		Schema:            h.fixture.schema, Decision: decision, Action: action, Status: ActionPrepared,
	}
	h.unresolved = &record
	return record.Clone(), nil
}

func (h *runtimeHarness) PrepareAction(_ context.Context, command PrepareActionCommand) (ActionRecord, error) {
	return h.prepareAction(command)
}

func (h *runtimeHarness) MarkDispatched(_ context.Context, command ActionMutation) (ActionRecord, error) {
	h.order = append(h.order, "dispatch")
	if h.unresolved == nil || h.unresolved.Action.ID != command.ActionID {
		return ActionRecord{}, errors.New("missing action")
	}
	h.unresolved.Status = ActionDispatched
	record := h.unresolved.Clone()
	if h.dispatchCommitError {
		h.dispatchCommitError = false
		return ActionRecord{}, errors.New("injected committed dispatch failure")
	}
	return record, nil
}

func seedUnresolvedAction(t *testing.T, h *runtimeHarness, binding Binding, status ActionStatus) ActionRecord {
	t.Helper()
	prepared, err := h.PrepareSnapshot(context.Background(), binding)
	requireNoError(t, err)
	outcome, err := h.Decide(context.Background(), prepared.Snapshot)
	requireNoError(t, err)
	decision := outcome.Decision
	action, err := cognition.NewRegisteredAction(
		"action-recovery-1", binding.Attempt, h.fixture.schema, decision.Action, decision.EvidenceRefs,
	)
	requireNoError(t, err)
	record := ActionRecord{
		Episode: binding.Episode, ExpectedRevision: prepared.Snapshot.CurrentRevision(),
		SnapshotSHA256: prepared.Snapshot.SHA256(), ContextProjection: prepared.Snapshot.ContextProjection(),
		Schema: h.fixture.schema, Decision: decision, Action: action, Status: status,
	}
	h.unresolved = &record
	h.order = nil
	h.policyCalls = 0
	return record.Clone()
}
