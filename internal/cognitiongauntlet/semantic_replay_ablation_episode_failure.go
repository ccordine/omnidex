package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func consumeAblationTraceFailure(
	cursor *ablationEpisodeTraceCursor,
	evidence ablationEvidenceArtifact,
) error {
	wantID, wantMessage, required, err := expectedAblationTraceFailure(evidence)
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	entry, err := cursor.next(TraceFailure)
	if err != nil {
		return err
	}
	root := evidence.Root
	var actual ablationFailureRecord
	if entry.ID != wantID || entry.Revision == nil || *entry.Revision != root.Terminal.Revision ||
		decodeTracePayload(entry.Payload, &actual, "ablation failure trace") != nil ||
		actual.Code != root.Terminal.FailureCode || actual.Message != wantMessage {
		return fmt.Errorf(
			"sealed ablation failure differs from exact terminal cause: id=%q want=%q code=%q want=%q message=%q want=%q",
			entry.ID, wantID, actual.Code, root.Terminal.FailureCode, actual.Message, wantMessage,
		)
	}
	return nil
}

func expectedAblationTraceFailure(
	evidence ablationEvidenceArtifact,
) (string, string, bool, error) {
	root := evidence.Root
	cause := root.TerminalCause
	switch cause.Kind {
	case ablationTerminalWorld, ablationTerminalActionFailure:
		return "", "", false, nil
	case ablationTerminalPreCallBudget:
		return "ablation-budget-model-calls",
			"The frozen model-call budget was exhausted.", true, nil
	case ablationTerminalCycleBudget:
		return "ablation-budget-cycles",
			"The frozen runtime-cycle budget was exhausted.", true, nil
	case ablationTerminalContextBudget:
		if root.ContextBudget == nil {
			return "", "", false, fmt.Errorf("context terminal lacks exact failed projection")
		}
		failure := &ablationContextBudgetFailure{
			projection:      root.ContextBudget.Projection.Projection,
			snapshot:        root.ContextBudget.Snapshot,
			modelInputBytes: root.ContextBudget.ModelInputBytes,
		}
		return fmt.Sprintf("ablation-context-budget-%03d", cause.CompletedCycles+1),
			failure.Error(), true, nil
	case ablationTerminalPolicyDecision:
		if cause.CallOrdinal == 0 || int(cause.CallOrdinal) > len(root.Calls) {
			return "", "", false, fmt.Errorf("policy terminal call is outside evidence")
		}
		result := root.Calls[cause.CallOrdinal-1].Result
		return fmt.Sprintf("ablation-policy-failure-%03d", cause.CallOrdinal),
			result.FailureMessage, true, nil
	case ablationTerminalNoDispatch:
		return expectedAblationNoDispatchFailure(evidence)
	default:
		return "", "", false, fmt.Errorf("ablation terminal cause has no trace derivation")
	}
}

func expectedAblationNoDispatchFailure(
	evidence ablationEvidenceArtifact,
) (string, string, bool, error) {
	root := evidence.Root
	cause := root.TerminalCause
	switch cause.Reason {
	case "resource_budget":
		return fmt.Sprintf("ablation-action-budget-%03d", cause.CallOrdinal),
			"The frozen environment-action budget was exhausted.", true, nil
	case "raw_shell_parse_failure":
		store, err := newAblationEvidenceContentStore(evidence)
		if err != nil {
			return "", "", false, err
		}
		role := cognitionreplay.ChunkedBlobPublicAgentKnowledge
		decisions, err := rederiveAblationDecisions(root.Calls, store, role)
		if err != nil {
			return "", "", false, err
		}
		call := root.Calls[cause.CallOrdinal-1]
		decision := decisions[call.Attempt.ID]
		if decision == nil {
			return "", "", false, fmt.Errorf("raw-shell terminal lacks its accepted decision")
		}
		_, parseErr := parseRawShellDecision(decision.Action, root.WorldCatalog)
		if parseErr == nil {
			return "", "", false, fmt.Errorf("raw-shell terminal reparses successfully")
		}
		return fmt.Sprintf("ablation-shell-failure-%03d", cause.CallOrdinal),
			parseErr.Error(), true, nil
	default:
		return "", "", false, fmt.Errorf("accepted no-dispatch cause is not registered")
	}
}
