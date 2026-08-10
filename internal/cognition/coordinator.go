package cognition

import (
	"context"
	"fmt"
	"reflect"
)

// Policy receives no Environment or mutation interface. It may propose exactly
// one bounded cognition decision from an immutable runtime snapshot.
type Policy interface {
	Decide(ctx context.Context, snapshot RuntimeSnapshot) (PolicyOutcome, error)
}

type PolicyOutcome struct {
	Decision          CognitionDecision
	InferenceExecuted bool
}

type PolicyFunc func(context.Context, RuntimeSnapshot) (PolicyOutcome, error)

func (function PolicyFunc) Decide(ctx context.Context, snapshot RuntimeSnapshot) (PolicyOutcome, error) {
	return function(ctx, snapshot)
}

type CoordinatorState string

const (
	CoordinatorActionReady         CoordinatorState = "action_ready"
	CoordinatorObligationSatisfied CoordinatorState = "obligation_satisfied"
)

type CoordinatorStep struct {
	State             CoordinatorState     `json:"state"`
	SnapshotSHA256    string               `json:"snapshot_sha256"`
	Decision          *CognitionDecision   `json:"decision,omitempty"`
	ActionSchema      ActionSchemaRef      `json:"action_schema,omitempty"`
	Actor             AttemptRef           `json:"actor"`
	ContextProjection ContextProjectionRef `json:"context_projection"`
	Completion        CompletionResult     `json:"completion"`
	RemainingBudget   RuntimeBudget        `json:"remaining_budget"`
	PolicyCalled      bool                 `json:"policy_called"`
}

type Coordinator struct {
	policy Policy
}

func NewCoordinator(policy Policy) (*Coordinator, error) {
	if nilInterface(policy) {
		return nil, fmt.Errorf("%w: policy is nil", ErrPolicyUnavailable)
	}
	return &Coordinator{policy: policy}, nil
}

func (coordinator *Coordinator) Step(
	ctx context.Context,
	snapshot RuntimeSnapshot,
	completion CompletionResult,
	completionEvidence []EvidenceRef,
) (CoordinatorStep, error) {
	if coordinator == nil || nilInterface(coordinator.policy) {
		return CoordinatorStep{}, fmt.Errorf("%w: coordinator policy is unavailable", ErrPolicyUnavailable)
	}
	if ctx == nil {
		return CoordinatorStep{}, fmt.Errorf("%w: context is nil", ErrPolicyFailed)
	}
	if err := snapshot.Validate(); err != nil {
		return CoordinatorStep{}, err
	}
	current := snapshot.CurrentObligation()
	if err := completion.ValidateFor(current, snapshot.CurrentRevision(), completionEvidence); err != nil {
		return CoordinatorStep{}, err
	}
	budget := snapshot.Budget()
	base := CoordinatorStep{
		SnapshotSHA256: snapshot.SHA256(), Completion: completion.Clone(),
		Actor: snapshot.Attempt(), ContextProjection: snapshot.ContextProjection(),
		RemainingBudget: budget,
	}
	if completion.Outcome == CompletionSatisfied {
		base.State = CoordinatorObligationSatisfied
		return base, nil
	}
	if budget.RemainingPolicyCalls == 0 {
		return CoordinatorStep{}, ErrCoordinatorBudgetExhausted
	}
	outcome, err := coordinator.policy.Decide(ctx, snapshot)
	base.PolicyCalled = outcome.InferenceExecuted
	if err != nil {
		return base, fmt.Errorf("%w: %w", ErrPolicyFailed, err)
	}
	decision := outcome.Decision
	catalog := snapshot.ActionCatalog()
	schema, exists := catalog.Schema(decision.Action.Kind)
	if !exists {
		return base, fmt.Errorf("%w: action kind %q is absent from the bound catalog", ErrInvalidDecision, decision.Action.Kind)
	}
	if err := decision.Validate(schema); err != nil {
		return base, err
	}
	if decision.ObligationID != current.ID {
		return base, fmt.Errorf("%w: decision targets another obligation", ErrInvalidDecision)
	}
	available := make(map[string]struct{}, len(snapshot.EvidenceRefs()))
	for _, ref := range snapshot.EvidenceRefs() {
		available[evidenceIdentity(ref)] = struct{}{}
	}
	if err := requireAvailableEvidence(decision.EvidenceRefs, available, "decision"); err != nil {
		return base, err
	}
	if err := validateDecisionBudget(decision, budget); err != nil {
		return base, err
	}
	budget.RemainingPolicyCalls--
	accepted := decision.Clone()
	base.State = CoordinatorActionReady
	base.Decision = &accepted
	base.ActionSchema = schema.Ref()
	base.RemainingBudget = budget
	return base, nil
}

func validateDecisionBudget(decision CognitionDecision, budget RuntimeBudget) error {
	if len(decision.EvidenceRefs) > budget.MaxEvidenceRefs ||
		len(decision.Action.Arguments) > budget.MaxActionArguments ||
		len(decision.Proposals) > budget.MaxLedgerProposals ||
		len(decision.Attention) > budget.MaxAttentionRequests ||
		len(decision.ExpectedEffect) > budget.MaxExpectedEffectBytes {
		return fmt.Errorf("%w: decision exceeds the bound runtime budget", ErrInvalidDecision)
	}
	return nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
