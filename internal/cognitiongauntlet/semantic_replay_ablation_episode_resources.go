package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/workingset"
)

func verifyAblationSemanticEpisodeResources(
	episode SealedEpisode,
	root ablationEvidenceRoot,
) error {
	resources := episode.Manifest.Resources
	modelCalls, modelDecisions := 0, 0
	for _, call := range root.Calls {
		if call.Result.ProviderRequestDisposition == llm.ProviderRequestDispatched {
			modelCalls++
		}
		if call.Result.Status == cognitionpolicy.CallResultAccepted {
			modelDecisions++
		}
	}
	search, read := 0, 0
	for _, action := range root.Actions {
		switch action.Trace.Action.Request.Kind {
		case "search":
			search++
		case "read":
			read++
		}
	}
	peakWorkingSet, err := ablationEvidencePeakWorkingSetBytes(root)
	if err != nil {
		return err
	}
	if resources.ModelCalls != modelCalls || resources.ModelDecisions != modelDecisions ||
		resources.EnvironmentActions != len(root.Actions) ||
		resources.LowLevelTransitions != len(root.Transitions)-1 ||
		resources.ToolOperations != len(root.Actions) ||
		resources.SearchOperations != search || resources.ReadOperations != read ||
		resources.PeakWorkingSetBytes != peakWorkingSet ||
		resources.PolicyWallMilliseconds != 0 {
		return fmt.Errorf("sealed ablation resources differ from exact evidence")
	}
	wantPlanning := PlanningMetrics{ObligationsCreated: 1, PlanGenerations: 1}
	if root.Terminal.GoalSatisfied {
		wantPlanning.ObligationsCompleted = 1
	}
	if root.TerminalCause.Kind == ablationTerminalActionFailure {
		wantPlanning.InvalidActions = 1
	}
	if episode.Manifest.Planning != wantPlanning ||
		episode.Manifest.Memory != (MemoryMetrics{}) ||
		episode.Manifest.Recovery != (RecoveryMetrics{}) {
		return fmt.Errorf("sealed ablation aggregate metrics differ from exact evidence")
	}
	return nil
}

func ablationEvidencePeakWorkingSetBytes(root ablationEvidenceRoot) (int64, error) {
	if root.WorkingSet == nil {
		return 0, nil
	}
	set, err := workingset.Restore(root.WorkingSet.Initial)
	if err != nil {
		return 0, err
	}
	eventIndex := 0
	peak := int64(0)
	for _, transition := range root.Transitions {
		for range transition.Observations {
			if eventIndex >= len(root.WorkingSet.Events) {
				return 0, fmt.Errorf("Working Set evidence ended before transition observations")
			}
			event := root.WorkingSet.Events[eventIndex]
			command, err := workingset.DecodeCommand(event.CommandKind, event.Command)
			if err != nil {
				return 0, err
			}
			if _, err := set.Apply(command); err != nil {
				return 0, err
			}
			eventIndex++
		}
		resident := int64(set.Usage().ResidentBytes)
		if resident > peak {
			peak = resident
		}
	}
	if eventIndex != len(root.WorkingSet.Events) {
		return 0, fmt.Errorf("Working Set evidence contains events outside transitions")
	}
	return peak, nil
}
