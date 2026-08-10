package cognition

import (
	"encoding/json"
	"fmt"
)

type RuntimeBudget struct {
	RemainingPolicyCalls   uint32 `json:"remaining_policy_calls"`
	MaxInputBytes          int    `json:"max_input_bytes"`
	MaxInputTokens         int    `json:"max_input_tokens"`
	MaxOutputBytes         int    `json:"max_output_bytes"`
	MaxOutputTokens        int    `json:"max_output_tokens"`
	MaxEvidenceRefs        int    `json:"max_evidence_refs"`
	MaxActionArguments     int    `json:"max_action_arguments"`
	MaxLedgerProposals     int    `json:"max_ledger_proposals"`
	MaxAttentionRequests   int    `json:"max_attention_requests"`
	MaxExpectedEffectBytes int    `json:"max_expected_effect_bytes"`
}

func (budget RuntimeBudget) Validate() error {
	if budget.RemainingPolicyCalls > MaxPolicyCallsPerEpisode {
		return fmt.Errorf("%w: remaining policy calls exceed %d", ErrInvalidRuntimeBudget, MaxPolicyCallsPerEpisode)
	}
	if budget.MaxInputBytes < 1 || budget.MaxInputBytes > MaxPolicyInputBytes ||
		budget.MaxInputTokens < 1 || budget.MaxInputTokens > MaxPolicyInputTokens {
		return fmt.Errorf("%w: per-call input budget is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	if budget.MaxOutputBytes < 1 || budget.MaxOutputBytes > MaxPolicyOutputBytes ||
		budget.MaxOutputTokens < 1 || budget.MaxOutputTokens > MaxPolicyOutputTokens {
		return fmt.Errorf("%w: per-call output budget is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	if budget.MaxEvidenceRefs < 0 || budget.MaxEvidenceRefs > MaxEvidenceRefs {
		return fmt.Errorf("%w: evidence limit is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	if budget.MaxActionArguments < 0 || budget.MaxActionArguments > MaxActionArguments {
		return fmt.Errorf("%w: action argument limit is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	if budget.MaxLedgerProposals < 0 || budget.MaxLedgerProposals > MaxLedgerProposals {
		return fmt.Errorf("%w: proposal limit is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	if budget.MaxAttentionRequests < 0 || budget.MaxAttentionRequests > MaxAttentionRequests {
		return fmt.Errorf("%w: attention limit is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	if budget.MaxExpectedEffectBytes < 1 || budget.MaxExpectedEffectBytes > MaxExpectedEffectBytes {
		return fmt.Errorf("%w: expected-effect limit is outside registered bounds", ErrInvalidRuntimeBudget)
	}
	return nil
}

// RuntimeSnapshot is immutable outside this package. Every composite getter
// returns a defensive copy before policy code can inspect it.
type RuntimeSnapshot struct {
	goal              GoalExpression
	currentRevision   WorldRevision
	currentObligation Obligation
	actionCatalog     ActionCatalog
	attempt           AttemptRef
	contextProjection ContextProjectionRef
	budget            RuntimeBudget
	evidenceRefs      []EvidenceRef
	sha256            string
}

func NewRuntimeSnapshot(
	goal GoalExpression,
	currentRevision WorldRevision,
	currentObligation Obligation,
	actionCatalog ActionCatalog,
	attempt AttemptRef,
	contextProjection ContextProjectionRef,
	budget RuntimeBudget,
	evidenceRefs []EvidenceRef,
) (RuntimeSnapshot, error) {
	snapshot := RuntimeSnapshot{
		goal: goal.Clone(), currentRevision: currentRevision,
		currentObligation: currentObligation.Clone(), actionCatalog: actionCatalog.Clone(),
		attempt: attempt, contextProjection: contextProjection,
		budget: budget, evidenceRefs: cloneSlice(evidenceRefs),
	}
	snapshot.sha256 = runtimeSnapshotSHA256(snapshot)
	if err := snapshot.Validate(); err != nil {
		return RuntimeSnapshot{}, err
	}
	return snapshot, nil
}

func (snapshot RuntimeSnapshot) Validate() error {
	if err := snapshot.goal.Validate(); err != nil {
		return fmt.Errorf("%w: goal: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if err := snapshot.currentRevision.Validate(); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if err := snapshot.currentObligation.Validate(); err != nil {
		return fmt.Errorf("%w: obligation: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if snapshot.currentObligation.Status != ObligationActive {
		return fmt.Errorf("%w: current obligation must be active", ErrInvalidRuntimeSnapshot)
	}
	if err := snapshot.actionCatalog.Validate(); err != nil {
		return fmt.Errorf("%w: action catalog: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if err := snapshot.attempt.Validate(); err != nil {
		return fmt.Errorf("%w: attempt: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if err := snapshot.contextProjection.Validate(); err != nil {
		return fmt.Errorf("%w: context projection: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if err := snapshot.budget.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRuntimeSnapshot, err)
	}
	if len(snapshot.evidenceRefs) > snapshot.budget.MaxEvidenceRefs {
		return fmt.Errorf("%w: evidence packet exceeds its runtime budget", ErrInvalidRuntimeSnapshot)
	}
	if err := validateEvidenceRefs(snapshot.evidenceRefs); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRuntimeSnapshot, err)
	}
	available := make(map[string]struct{}, len(snapshot.evidenceRefs))
	for index, ref := range snapshot.evidenceRefs {
		if ref.Revision.EpisodeID != snapshot.currentRevision.EpisodeID ||
			ref.Revision.Number > snapshot.currentRevision.Number {
			return fmt.Errorf("%w: evidence %d is unavailable at the current revision", ErrInvalidRuntimeSnapshot, index)
		}
		available[evidenceIdentity(ref)] = struct{}{}
	}
	for index, ref := range snapshot.currentObligation.SupportingRefs {
		if _, exists := available[evidenceIdentity(ref)]; !exists {
			return fmt.Errorf("%w: obligation evidence %d is absent from the evidence packet", ErrInvalidRuntimeSnapshot, index)
		}
	}
	if !validSHA256(snapshot.sha256) || runtimeSnapshotSHA256(snapshot) != snapshot.sha256 {
		return fmt.Errorf("%w: hash does not bind the exact runtime snapshot", ErrInvalidRuntimeSnapshot)
	}
	return nil
}

func (snapshot RuntimeSnapshot) Goal() GoalExpression           { return snapshot.goal.Clone() }
func (snapshot RuntimeSnapshot) CurrentRevision() WorldRevision { return snapshot.currentRevision }
func (snapshot RuntimeSnapshot) CurrentObligation() Obligation {
	return snapshot.currentObligation.Clone()
}
func (snapshot RuntimeSnapshot) ActionCatalog() ActionCatalog { return snapshot.actionCatalog.Clone() }
func (snapshot RuntimeSnapshot) Attempt() AttemptRef          { return snapshot.attempt }
func (snapshot RuntimeSnapshot) ContextProjection() ContextProjectionRef {
	return snapshot.contextProjection
}
func (snapshot RuntimeSnapshot) Budget() RuntimeBudget { return snapshot.budget }
func (snapshot RuntimeSnapshot) EvidenceRefs() []EvidenceRef {
	return cloneSlice(snapshot.evidenceRefs)
}
func (snapshot RuntimeSnapshot) SHA256() string { return snapshot.sha256 }

func runtimeSnapshotSHA256(snapshot RuntimeSnapshot) string {
	payload := struct {
		Goal              GoalExpression       `json:"goal"`
		CurrentRevision   WorldRevision        `json:"current_revision"`
		CurrentObligation Obligation           `json:"current_obligation"`
		ActionCatalog     ActionCatalog        `json:"action_catalog"`
		Attempt           AttemptRef           `json:"attempt"`
		ContextProjection ContextProjectionRef `json:"context_projection"`
		Budget            RuntimeBudget        `json:"budget"`
		EvidenceRefs      []EvidenceRef        `json:"evidence_refs"`
	}{snapshot.goal, snapshot.currentRevision, snapshot.currentObligation,
		snapshot.actionCatalog, snapshot.attempt, snapshot.contextProjection,
		snapshot.budget, snapshot.evidenceRefs}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal runtime snapshot identity: %v", err))
	}
	return contentSHA256(string(raw))
}
