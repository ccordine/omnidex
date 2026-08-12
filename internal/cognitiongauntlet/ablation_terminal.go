package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type ablationPendingTerminal struct {
	Revision      cognition.WorldRevision `json:"revision"`
	PublicOutcome string                  `json:"public_outcome"`
	GoalSatisfied bool                    `json:"goal_satisfied"`
	FailureCode   string                  `json:"failure_code,omitempty"`
}

func setPendingAblationTerminal(
	execution *ablationExecution,
	revision cognition.WorldRevision,
	publicOutcome string,
	goalSatisfied bool,
	failureCode string,
) error {
	if execution == nil {
		return fmt.Errorf("ablation execution is nil")
	}
	if execution.Terminal != nil {
		return fmt.Errorf("ablation execution already has a terminal authority")
	}
	value := ablationPendingTerminal{
		Revision: revision, PublicOutcome: publicOutcome,
		GoalSatisfied: goalSatisfied, FailureCode: failureCode,
	}
	if err := value.Validate(); err != nil {
		return err
	}
	execution.Terminal = &value
	return nil
}

func setAblationTerminalCause(
	execution *ablationExecution,
	cause ablationTerminalCause,
) error {
	if execution == nil || execution.TerminalCause != nil {
		return fmt.Errorf("ablation execution already has a terminal cause")
	}
	if err := cause.Validate(); err != nil {
		return err
	}
	copy := cause
	execution.TerminalCause = &copy
	return nil
}

func (cause ablationTerminalCause) Validate() error {
	if cause.CompletedCalls < 0 || cause.CompletedCycles < 0 ||
		cause.CompletedCycles < cause.CompletedCalls {
		return fmt.Errorf("ablation terminal cause counters are invalid")
	}
	switch cause.Kind {
	case ablationTerminalWorld:
		if cause.CallOrdinal == 0 || cause.ActionID == "" || cause.Reason != "" {
			return fmt.Errorf("world terminal cause is invalid")
		}
	case ablationTerminalActionFailure:
		if cause.CallOrdinal == 0 || cause.ActionID == "" || cause.Reason == "" {
			return fmt.Errorf("action failure terminal cause is invalid")
		}
	case ablationTerminalPolicyDecision, ablationTerminalNoDispatch:
		if cause.CallOrdinal == 0 || cause.ActionID != "" || cause.Reason == "" {
			return fmt.Errorf("policy terminal cause is invalid")
		}
	case ablationTerminalPreCallBudget, ablationTerminalContextBudget, ablationTerminalCycleBudget:
		if cause.CallOrdinal != 0 || cause.ActionID != "" || cause.Reason == "" {
			return fmt.Errorf("budget terminal cause is invalid")
		}
	default:
		return fmt.Errorf("ablation terminal cause kind is not registered")
	}
	return nil
}

func (value ablationPendingTerminal) Validate() error {
	if err := value.Revision.Validate(); err != nil {
		return fmt.Errorf("ablation terminal revision: %w", err)
	}
	if err := requireExact(value.PublicOutcome, "ablation terminal public outcome", 4096); err != nil {
		return err
	}
	if value.GoalSatisfied {
		if value.FailureCode != "" {
			return fmt.Errorf("successful ablation terminal cannot claim failure")
		}
		return nil
	}
	if err := requireExact(value.FailureCode, "ablation terminal failure code", 256); err != nil {
		return err
	}
	if value.PublicOutcome != value.FailureCode {
		return fmt.Errorf("failed ablation terminal outcome differs from failure code")
	}
	return nil
}

func appendPendingAblationTerminal(
	recorder *EpisodeRecorder,
	execution ablationExecution,
) error {
	if execution.Terminal == nil || execution.Terminal.Validate() != nil ||
		execution.Terminal.Revision != execution.Revision ||
		execution.Terminal.PublicOutcome != execution.Outcome.PublicOutcome ||
		execution.Terminal.GoalSatisfied != execution.Outcome.GoalSatisfied ||
		execution.Terminal.FailureCode != execution.Outcome.FailureCode ||
		!execution.Outcome.Terminal {
		return fmt.Errorf("ablation terminal authority differs from execution outcome")
	}
	return appendAblationTerminal(
		recorder, execution.Terminal.Revision, execution.Terminal.PublicOutcome,
		execution.Terminal.GoalSatisfied,
	)
}
