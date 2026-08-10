package cognitionruntime

import "github.com/gryph/omnidex/internal/cognition"

type StepState string

const (
	StepActionSucceeded    StepState = "action_succeeded"
	StepActionFailed       StepState = "action_failed"
	StepObligationAdvanced StepState = "obligation_advanced"
	StepEpisodeCompleted   StepState = "episode_completed"
	StepEpisodeFailed      StepState = "episode_failed"
	StepEpisodeCanceled    StepState = "episode_canceled"
)

type StepResult struct {
	State                  StepState                   `json:"state"`
	Binding                Binding                     `json:"binding"`
	Revision               cognition.WorldRevision     `json:"revision"`
	ActionID               cognition.ActionID          `json:"action_id,omitempty"`
	Transition             *cognition.Transition       `json:"transition,omitempty"`
	Failure                *cognition.ActionFailure    `json:"failure,omitempty"`
	Completion             *cognition.CompletionResult `json:"completion,omitempty"`
	Seal                   *TerminalSeal               `json:"seal,omitempty"`
	Cancellation           *CancellationSeal           `json:"cancellation,omitempty"`
	PolicyCalled           bool                        `json:"policy_called"`
	RecoveredDecision      bool                        `json:"recovered_decision"`
	RecoveredAction        bool                        `json:"recovered_action"`
	RecoveredProgress      bool                        `json:"recovered_progress"`
	RecoveredPolicyOutcome bool                        `json:"recovered_policy_outcome"`
	PolicyCallAbandonment  *PolicyCallAbandonmentRef   `json:"policy_call_abandonment,omitempty"`
	AbandonedPolicyCalls   uint32                      `json:"abandoned_policy_calls"`
	EnvironmentActions     uint32                      `json:"environment_actions"`
}

type RunLimits struct {
	MaxCycles uint32 `json:"max_cycles"`
}

type RunResult struct {
	Terminal                StepResult `json:"terminal"`
	Cycles                  uint32     `json:"cycles"`
	PolicyCalls             uint32     `json:"policy_calls"`
	RecoveredDecisions      uint32     `json:"recovered_decisions"`
	RecoveredActions        uint32     `json:"recovered_actions"`
	RecoveredProgress       uint32     `json:"recovered_progress"`
	RecoveredPolicyOutcomes uint32     `json:"recovered_policy_outcomes"`
	AbandonedPolicyCalls    uint32     `json:"abandoned_policy_calls"`
	EnvironmentActions      uint32     `json:"environment_actions"`
}
