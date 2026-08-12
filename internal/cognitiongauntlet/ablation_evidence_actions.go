package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func verifyAblationActionEvidence(
	root ablationEvidenceRoot,
	decisions map[string]*cognition.CognitionDecision,
) error {
	if root.Actions == nil || root.NoActions == nil ||
		len(root.Actions)+len(root.NoActions) != len(root.Calls) {
		return fmt.Errorf("ablation evidence must account for every call exactly once")
	}
	actions, noActions, err := indexAblationActionEvidence(root)
	if err != nil {
		return err
	}
	current := root.Transitions[0].Current
	transitionIndex := 1
	terminalBound := false
	for index, call := range root.Calls {
		ordinal := uint32(index + 1)
		if call.Attempt.ExpectedRevision != current || call.Ordinal != ordinal {
			return fmt.Errorf("ablation call %d is not bound to the current revision", ordinal)
		}
		decision, exists := decisions[call.Attempt.ID]
		if !exists {
			return fmt.Errorf("ablation call %d lacks its rederived outcome", ordinal)
		}
		if action, found := actions[ordinal]; found {
			if decision == nil {
				return fmt.Errorf("ablation action %d lacks an accepted decision", ordinal)
			}
			advanced, err := verifyOneAblationAction(
				root, call, *decision, action, current, transitionIndex,
			)
			if err != nil {
				return err
			}
			if action.Trace.Transition != nil {
				current, transitionIndex = advanced, transitionIndex+1
			} else if ordinal != uint32(len(root.Calls)) ||
				root.Terminal.Revision != current ||
				root.Terminal.FailureCode != string(action.Trace.Failure.Code) ||
				root.Terminal.PublicOutcome != string(action.Trace.Failure.Code) ||
				root.TerminalCause != (ablationTerminalCause{
					Kind: ablationTerminalActionFailure, CallOrdinal: ordinal,
					ActionID: action.Trace.Action.ID, Reason: string(action.Trace.Failure.Code),
					CompletedCalls: len(root.Calls), CompletedCycles: int(ordinal),
				}) {
				return fmt.Errorf("ablation failed action differs from terminal authority")
			} else {
				terminalBound = true
			}
			continue
		}
		noAction, found := noActions[ordinal]
		if !found || noAction.CallID != call.Attempt.ID ||
			ordinal != uint32(len(root.Calls)) {
			return fmt.Errorf("ablation call %d disposition is missing or nonterminal", ordinal)
		}
		if err := verifyAblationNoAction(
			root, actions, call, decision, noAction, current,
		); err != nil {
			return fmt.Errorf("ablation call %d: %w", ordinal, err)
		}
		terminalBound = true
	}
	if transitionIndex != len(root.Transitions) || current != root.Terminal.Revision {
		return fmt.Errorf("ablation action transitions do not close at the terminal revision")
	}
	return verifyAblationTerminalCause(root, terminalBound)
}

func indexAblationActionEvidence(
	root ablationEvidenceRoot,
) (map[uint32]ablationActionEvidence, map[uint32]ablationNoActionEvidence, error) {
	actions := make(map[uint32]ablationActionEvidence, len(root.Actions))
	previous := uint32(0)
	for _, value := range root.Actions {
		if value.Cycle <= previous || value.Cycle == 0 || value.CallID == "" {
			return nil, nil, fmt.Errorf("ablation action order is invalid")
		}
		actions[value.Cycle], previous = value, value.Cycle
	}
	noActions := make(map[uint32]ablationNoActionEvidence, len(root.NoActions))
	previous = 0
	for _, value := range root.NoActions {
		if value.Cycle <= previous || value.Cycle == 0 || value.CallID == "" {
			return nil, nil, fmt.Errorf("ablation no-action order is invalid")
		}
		if _, duplicate := actions[value.Cycle]; duplicate {
			return nil, nil, fmt.Errorf("ablation call has two dispositions")
		}
		noActions[value.Cycle], previous = value, value.Cycle
	}
	return actions, noActions, nil
}

func verifyOneAblationAction(
	root ablationEvidenceRoot,
	call ablationCallEvidence,
	decision cognition.CognitionDecision,
	value ablationActionEvidence,
	current cognition.WorldRevision,
	transitionIndex int,
) (cognition.WorldRevision, error) {
	if value.CallID != call.Attempt.ID || value.Trace.Schema != ActionTraceSchemaV1 ||
		value.Trace.ExpectedRevision != current ||
		(value.Trace.Transition == nil) == (value.Trace.Failure == nil) {
		return cognition.WorldRevision{}, fmt.Errorf("ablation action authority is invalid")
	}
	request := decision.Action
	if root.Variant == VariantRawShell {
		parsed, err := parseRawShellDecision(request, root.WorldCatalog)
		if err != nil {
			return cognition.WorldRevision{}, fmt.Errorf("reparse raw-shell action: %w", err)
		}
		request = parsed
	}
	catalog := call.Snapshot.ActionCatalog
	if root.Variant == VariantRawShell {
		catalog = root.WorldCatalog
	}
	schema, exists := catalog.Schema(request.Kind)
	if !exists {
		return cognition.WorldRevision{}, fmt.Errorf("ablation action schema disappeared")
	}
	action, err := newAblationRegisteredAction(
		cognition.EpisodeRef{ID: root.EpisodeID}, call.Attempt.Actor,
		schema, value.Cycle, decision, request,
	)
	if err != nil || !reflect.DeepEqual(action, value.Trace.Action) {
		return cognition.WorldRevision{}, fmt.Errorf("ablation action differs from decision: %v", err)
	}
	if value.Trace.Transition != nil {
		if transitionIndex >= len(root.Transitions) ||
			!reflect.DeepEqual(*value.Trace.Transition, root.Transitions[transitionIndex]) ||
			value.Trace.Transition.ValidateApply(
				cognition.EpisodeRef{ID: root.EpisodeID}, current, action,
			) != nil {
			return cognition.WorldRevision{}, fmt.Errorf("ablation action transition changed")
		}
		return value.Trace.Transition.Current, nil
	}
	if value.Trace.Failure.Validate(action, current) != nil {
		return cognition.WorldRevision{}, fmt.Errorf("ablation action failure changed")
	}
	return current, nil
}

func verifyAblationNoAction(
	root ablationEvidenceRoot,
	actions map[uint32]ablationActionEvidence,
	call ablationCallEvidence,
	decision *cognition.CognitionDecision,
	value ablationNoActionEvidence,
	current cognition.WorldRevision,
) error {
	terminal := root.Terminal
	if terminal.GoalSatisfied || terminal.Revision != current ||
		requireExact(value.Reason, "ablation no-action reason", 256) != nil {
		return fmt.Errorf("no-action terminal authority is invalid")
	}
	wantTerminal := "model_policy"
	switch value.Kind {
	case ablationPolicyNoDecision:
		if decision != nil || value.Reason != string(call.Result.FailureCode) {
			return fmt.Errorf("policy no-action differs from rejected result")
		}
		if call.Result.FailureCode == cognitionpolicy.CallFailureProviderUsageLimit {
			wantTerminal = "resource_budget"
		}
	case ablationAcceptedNoAction:
		if decision == nil {
			return fmt.Errorf("accepted no-action lacks its decision")
		}
		switch value.Reason {
		case "resource_budget":
			if len(actions) < root.PublicRunAuthority.Budget.EnvironmentActions &&
				len(actions) < root.PublicRunAuthority.Budget.ToolOperations {
				return fmt.Errorf("accepted no-action falsely claims exhausted action budget")
			}
			wantTerminal = "resource_budget"
		case "raw_shell_parse_failure":
			if root.Variant != VariantRawShell {
				return fmt.Errorf("raw-shell failure belongs to another variant")
			}
			if _, err := parseRawShellDecision(decision.Action, root.WorldCatalog); err == nil {
				return fmt.Errorf("raw-shell failure claim reparses successfully")
			}
		default:
			return fmt.Errorf("accepted no-action reason is not registered")
		}
	default:
		return fmt.Errorf("no-action disposition is not registered")
	}
	if terminal.FailureCode != wantTerminal || terminal.PublicOutcome != wantTerminal {
		return fmt.Errorf("no-action disposition differs from terminal failure")
	}
	wantCause := ablationTerminalCause{
		CallOrdinal: value.Cycle, Reason: value.Reason,
		CompletedCalls: len(root.Calls), CompletedCycles: int(value.Cycle),
	}
	if value.Kind == ablationPolicyNoDecision {
		wantCause.Kind = ablationTerminalPolicyDecision
	} else {
		wantCause.Kind = ablationTerminalNoDispatch
	}
	if root.TerminalCause != wantCause {
		return fmt.Errorf("no-action disposition differs from terminal cause")
	}
	return nil
}

func verifyAblationTerminalCause(root ablationEvidenceRoot, terminalBound bool) error {
	if root.Terminal.GoalSatisfied {
		last := root.Transitions[len(root.Transitions)-1]
		want := ablationTerminalCause{
			Kind: ablationTerminalWorld, CallOrdinal: uint32(len(root.Calls)),
			ActionID: last.ActionID, CompletedCalls: len(root.Calls),
			CompletedCycles: len(root.Calls),
		}
		if terminalBound || root.TerminalCause != want {
			return fmt.Errorf("successful world terminal cause is inexact")
		}
		return nil
	}
	if terminalBound {
		return nil
	}
	cause := root.TerminalCause
	if root.Terminal.FailureCode != "resource_budget" ||
		root.Terminal.PublicOutcome != "resource_budget" ||
		cause.CompletedCalls != len(root.Calls) || cause.CompletedCycles != len(root.Calls) {
		return fmt.Errorf("unbound ablation terminal is not an exact budget failure")
	}
	switch cause.Kind {
	case ablationTerminalPreCallBudget:
		if cause.Reason != "model_calls" ||
			len(root.Calls) != root.PublicRunAuthority.Budget.ModelCalls ||
			len(root.Calls) >= root.PublicRunAuthority.Budget.RuntimeCycles {
			return fmt.Errorf("pre-call budget cause differs from frozen model-call limit")
		}
	case ablationTerminalCycleBudget:
		if cause.Reason != "runtime_cycles" ||
			len(root.Calls) != root.PublicRunAuthority.Budget.RuntimeCycles ||
			len(root.Calls) > root.PublicRunAuthority.Budget.ModelCalls {
			return fmt.Errorf("cycle budget cause differs from frozen runtime limit")
		}
	case ablationTerminalContextBudget:
		if cause.Reason != "context_projection" || root.ContextBudget == nil ||
			len(root.Calls) >= root.PublicRunAuthority.Budget.ModelCalls ||
			len(root.Calls) >= root.PublicRunAuthority.Budget.RuntimeCycles {
			return fmt.Errorf("context budget cause lacks its exact failed projection")
		}
	default:
		return fmt.Errorf("ablation terminal cause is not bound to its evidence")
	}
	return nil
}
