package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *productionTraceState) measureGraph(graph cognition.ObligationGraphSnapshot) {
	planning := &state.metrics.Planning
	if len(graph.Obligations) > planning.ObligationsCreated {
		planning.ObligationsCreated = len(graph.Obligations)
	}
	if int(graph.Generation) > planning.PlanGenerations {
		planning.PlanGenerations = int(graph.Generation)
	}
	completed := 0
	for _, obligation := range graph.Obligations {
		if obligation.Status == cognition.ObligationSatisfied {
			completed++
		}
	}
	if completed > planning.ObligationsCompleted {
		planning.ObligationsCompleted = completed
	}
}

func (state *productionTraceState) finish(
	recorder *EpisodeRecorder,
	restarts []RestartTrace,
) (productionTraceMetrics, error) {
	if err := state.diagnostics.finish(state); err != nil {
		return productionTraceMetrics{}, err
	}
	for id := range state.actions {
		if _, consumed := state.consumedActions[id]; !consumed {
			return productionTraceMetrics{}, fmt.Errorf(
				"sealed production action %q lacks its exact transition or failure trace", id,
			)
		}
	}
	if len(state.attempts) != state.metrics.Resources.ModelCalls {
		return productionTraceMetrics{}, fmt.Errorf("sealed production policy attempts and results differ")
	}
	if len(restarts) != state.metrics.Recovery.Restarts {
		return productionTraceMetrics{}, fmt.Errorf("actual restart receipts do not match recovery metrics")
	}
	for _, restart := range restarts {
		if err := restart.Validate(); err != nil {
			return productionTraceMetrics{}, err
		}
		payload, err := traceJSONObject(restart)
		if err != nil {
			return productionTraceMetrics{}, err
		}
		if err := recorder.Append(TraceRestart, restart.ID, nil, payload); err != nil {
			return productionTraceMetrics{}, err
		}
	}
	seal := state.trace.Header.Seal
	completed := seal.Outcome == queue.CognitionEpisodeCompleted
	failed := seal.Outcome == queue.CognitionEpisodeFailed
	canceled := seal.Outcome == queue.CognitionEpisodeCanceled
	if !completed && !failed && !canceled {
		return productionTraceMetrics{}, fmt.Errorf("sealed production trace has an unsupported terminal outcome")
	}
	if canceled {
		return state.finishCanceled(recorder, seal.FinalRevision)
	}
	if state.terminalProgress == nil {
		return productionTraceMetrics{}, fmt.Errorf("sealed production trace lacks terminal progress")
	}
	if (completed && state.terminalProgress.State != "completed") ||
		(failed && state.terminalProgress.State != "failed") ||
		state.terminalProgress.Revision != seal.FinalRevision ||
		state.terminalProgress.PublicOutcome == "" {
		return productionTraceMetrics{}, fmt.Errorf("sealed production terminal progress changed")
	}
	payload, err := traceJSONObject(oracleTerminalTrace{
		Revision: seal.FinalRevision, PublicOutcome: state.terminalProgress.PublicOutcome,
		GoalSatisfied: completed,
	})
	if err != nil {
		return productionTraceMetrics{}, err
	}
	if err := recorder.Append(
		TraceTerminal, "terminal-"+state.trace.Header.TraceSHA256,
		&seal.FinalRevision, payload,
	); err != nil {
		return productionTraceMetrics{}, err
	}
	state.metrics.Outcome = Outcome{
		Terminal: true, GoalSatisfied: completed,
		PublicOutcome: state.terminalProgress.PublicOutcome,
	}
	if failed {
		state.metrics.Outcome.FailureCode = string(seal.Outcome)
	}
	return state.metrics, nil
}

func (state *productionTraceState) finishCanceled(
	recorder *EpisodeRecorder,
	revision cognition.WorldRevision,
) (productionTraceMetrics, error) {
	if state.terminalProgress != nil || state.cancellation == nil {
		return productionTraceMetrics{}, fmt.Errorf("canceled production trace lacks exact cancellation evidence")
	}
	payload, err := traceJSONObject(oracleTerminalTrace{
		Revision: revision, PublicOutcome: state.cancellation.PublicMessage, GoalSatisfied: false,
	})
	if err != nil {
		return productionTraceMetrics{}, err
	}
	if err := recorder.Append(
		TraceTerminal, "terminal-"+state.trace.Header.TraceSHA256, &revision, payload,
	); err != nil {
		return productionTraceMetrics{}, err
	}
	state.metrics.Outcome = Outcome{
		Terminal: true, GoalSatisfied: false, PublicOutcome: state.cancellation.PublicMessage,
		FailureCode: string(state.cancellation.Code),
	}
	return state.metrics, nil
}
