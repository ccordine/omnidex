package cognitionruntime

import (
	"context"

	"github.com/gryph/omnidex/internal/cognition"
)

// SnapshotPreparer restores authoritative state, applies the registered
// working-set policy, and creates one immutable bounded projection. Projection
// capacity is independent from the episode's remaining-call allowance.
type SnapshotPreparer interface {
	PrepareSnapshot(context.Context, Binding) (PreparedSnapshot, error)
}

// AcceptedDecisionJournal returns code-authorized recovery only when an exact
// accepted policy decision is stranded before action preparation. A nil result
// means no such decision exists for the current replacement attempt.
type AcceptedDecisionJournal interface {
	RecoverAccepted(context.Context, Binding) (*AcceptedDecisionRecovery, error)
}

// PolicyRecoveryJournal prevents a durable terminal or indeterminate policy
// call from being silently replaced by another inference. Terminal outcomes
// are replayed as their registered policy error. An indeterminate call may be
// abandoned only by exact replacement-attempt authority; the consumed call is
// never refunded.
type PolicyRecoveryJournal interface {
	ReplayTerminalPolicyOutcome(context.Context, Binding) (bool, error)
	AbandonIndeterminate(context.Context, Binding) (*PolicyCallAbandonment, error)
}

// CompletionEvaluator is code authority. It evaluates a registered predicate
// and cannot receive a model decision.
type CompletionEvaluator interface {
	Evaluate(context.Context, CompletionRequest) (cognition.CompletionResult, error)
}

// EpisodeJournal owns obligation progress derived from completion results.
// AdvanceSatisfied must atomically persist satisfaction, refresh readiness,
// and activate exactly one next obligation unless the graph became terminal.
// FailTerminal must atomically fail the graph root from a terminal environment
// state. Both methods must provide exact replay for the same command.
type EpisodeJournal interface {
	TerminalProgress(context.Context, Binding) (*EpisodeProgress, error)
	AdvanceSatisfied(context.Context, CompletionCommand) (EpisodeProgress, error)
	FailTerminal(context.Context, CompletionCommand) (EpisodeProgress, error)
}

// DecisionReconciler records bounded model proposals and treats attention as
// advisory under the registered state policy. Its receipt binds the resulting
// ledger and working-set versions.
type DecisionReconciler interface {
	Reconcile(context.Context, ReconciliationCommand) (ReconciliationReceipt, error)
}

// ActionJournal is the only authority that assigns an ActionID. PrepareAction
// must verify immutable policy evidence and the reconciliation versions. Every
// method must be idempotent for identical content and reject changed replay
// content.
type ActionJournal interface {
	Unresolved(context.Context, Binding) (*ActionRecord, error)
	PrepareAction(context.Context, PrepareActionCommand) (ActionRecord, error)
	MarkDispatched(context.Context, ActionMutation) (ActionRecord, error)
	RecordFailure(context.Context, FailureMutation) (ActionRecord, error)
	RecordTransition(context.Context, TransitionMutation) (ActionRecord, error)
}

// TerminalSealer persists the immutable final trace after code-owned progress
// reaches a terminal graph state.
type TerminalSealer interface {
	Seal(context.Context, SealCommand) (TerminalSeal, error)
}

type Dependencies struct {
	Policy         cognition.Policy
	Environment    cognition.Environment
	Snapshots      SnapshotPreparer
	Accepted       AcceptedDecisionJournal
	PolicyRecovery PolicyRecoveryJournal
	Completion     CompletionEvaluator
	Episodes       EpisodeJournal
	Reconciler     DecisionReconciler
	Actions        ActionJournal
	TerminalSeal   TerminalSealer
}
