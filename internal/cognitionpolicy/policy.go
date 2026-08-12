package cognitionpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
)

type Policy struct {
	client      llm.Client
	exactClient llm.ExactPreparedContractClient
	brain       AttestedBrain
	activation  ProviderProcessActivationAuthority
	projections ProjectionLoader
	journal     CallJournal
}

type ProjectionLoader interface {
	LoadProjection(context.Context, cognition.ContextProjectionRef) (contextbuilder.Projection, error)
}

var _ cognition.Policy = (*Policy)(nil)

func New(
	client llm.Client,
	brain AttestedBrain,
	activation ProviderProcessActivationAuthority,
	projections ProjectionLoader,
	journal CallJournal,
) (*Policy, error) {
	if nilPolicyDependency(client) {
		return nil, fmt.Errorf("%w: LLM client is nil", ErrInvalidConfig)
	}
	if nilPolicyDependency(journal) {
		return nil, fmt.Errorf("%w: call journal is nil", ErrInvalidConfig)
	}
	if nilPolicyDependency(projections) {
		return nil, fmt.Errorf("%w: projection loader is nil", ErrInvalidConfig)
	}
	if err := brain.Validate(); err != nil {
		return nil, err
	}
	if err := activation.Validate(); err != nil {
		return nil, err
	}
	exactClient, err := llm.RequireExactPreparedContract(client)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	expected, err := brain.Ref.ProviderExpectation()
	if err != nil {
		return nil, err
	}
	if err := exactClient.ValidateExactPreparedProvider(expected); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return &Policy{
		client: client, exactClient: exactClient, brain: brain, activation: activation,
		projections: projections, journal: journal,
	}, nil
}

func (policy *Policy) Decide(
	ctx context.Context,
	snapshot cognition.RuntimeSnapshot,
) (cognition.PolicyOutcome, error) {
	if policy == nil || nilPolicyDependency(policy.client) ||
		nilPolicyDependency(policy.exactClient) ||
		nilPolicyDependency(policy.projections) || nilPolicyDependency(policy.journal) {
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: policy is uninitialized", ErrInvalidConfig)
	}
	if ctx == nil {
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: context is nil", ErrInvalidConfig)
	}
	if err := snapshot.Validate(); err != nil {
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: snapshot: %v", ErrInvalidConfig, err)
	}
	projection, err := policy.projections.LoadProjection(ctx, snapshot.ContextProjection())
	if err != nil {
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: %w", ErrInvalidProjection, err)
	}
	if err := projection.Validate(); err != nil {
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: %v", ErrInvalidProjection, err)
	}
	projection = cloneProjection(projection)
	envelope, err := Render(snapshot, projection, policy.brain.Ref)
	if err != nil {
		return cognition.PolicyOutcome{}, err
	}
	stable, err := policy.brain.StableAuthority()
	if err != nil {
		return cognition.PolicyOutcome{}, err
	}
	if err := policy.activation.ValidateFor(
		stable, snapshot.CurrentRevision().EpisodeID, snapshot.Attempt(),
	); err != nil {
		return cognition.PolicyOutcome{}, err
	}
	attempt, err := newCallAttempt(snapshot, policy.brain, policy.activation, envelope)
	if err != nil {
		return cognition.PolicyOutcome{}, err
	}
	reservation, err := policy.journal.Start(ctx, attempt)
	if err != nil {
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: reserve call: %v", ErrCallJournal, err)
	}
	if err := reservation.ValidateFor(attempt); err != nil {
		return cognition.PolicyOutcome{}, err
	}
	if !reservation.Created {
		return policy.replayReservedCall(reservation, snapshot)
	}
	return policy.executeReservedCall(ctx, attempt, snapshot)
}

func responseActionKind(response string) (cognition.ActionKind, error) {
	if err := cognition.ValidateCognitionDecisionAuthority([]byte(response)); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidDecision, err)
	}
	var route struct {
		Action struct {
			Kind cognition.ActionKind `json:"kind"`
		} `json:"action"`
	}
	if err := json.Unmarshal([]byte(response), &route); err != nil {
		return "", fmt.Errorf("%w: decode action route: %v", ErrInvalidDecision, err)
	}
	if route.Action.Kind == "" {
		return "", fmt.Errorf("%w: response action kind is required", ErrInvalidDecision)
	}
	return route.Action.Kind, nil
}

func validateDecisionForSnapshot(
	decision cognition.CognitionDecision,
	snapshot cognition.RuntimeSnapshot,
) error {
	if decision.ObligationID != snapshot.CurrentObligation().ID {
		return fmt.Errorf("%w: decision targets another obligation", ErrInvalidDecision)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(snapshot.EvidenceRefs()))
	for _, ref := range snapshot.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for index, ref := range decision.EvidenceRefs {
		if _, exists := available[ref]; !exists {
			return fmt.Errorf("%w: evidence %d is absent from the bound snapshot", ErrInvalidDecision, index)
		}
	}
	budget := snapshot.Budget()
	if budget.RemainingPolicyCalls == 0 || len(decision.EvidenceRefs) > budget.MaxEvidenceRefs ||
		len(decision.Action.Arguments) > budget.MaxActionArguments ||
		len(decision.Proposals) > budget.MaxLedgerProposals ||
		len(decision.Attention) > budget.MaxAttentionRequests ||
		len(decision.ExpectedEffect) > budget.MaxExpectedEffectBytes {
		return fmt.Errorf("%w: decision exceeds the bound runtime budget", ErrInvalidDecision)
	}
	return nil
}

func cloneProjection(projection contextbuilder.Projection) contextbuilder.Projection {
	projection.Selected = append(make([]contextbuilder.Selection, 0, len(projection.Selected)), projection.Selected...)
	for index := range projection.Selected {
		projection.Selected[index].SourceRefs = slices.Clone(projection.Selected[index].SourceRefs)
	}
	projection.Omitted = append(make([]contextbuilder.Omission, 0, len(projection.Omitted)), projection.Omitted...)
	return projection
}

func nilPolicyDependency(value any) bool {
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
