package cognitionruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

func (h *runtimeHarness) Apply(
	_ context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	h.order = append(h.order, "environment")
	h.environmentCalls++
	h.applied = append(h.applied, action.Clone())
	if h.environmentError != nil {
		return cognition.Transition{}, h.environmentError
	}
	if h.typedFailureCode != "" {
		failure, err := cognition.NewActionFailure(
			h.typedFailureCode, action, expected, "The registered operation was rejected.", action.EvidenceRefs,
		)
		if err != nil {
			return cognition.Transition{}, err
		}
		return cognition.Transition{}, failure
	}
	if transition, exists := h.receipts[action.ID]; exists {
		return transition.Clone(), nil
	}
	if episode != h.fixture.binding.Episode || expected != h.env {
		return cognition.Transition{}, fmt.Errorf("unexpected environment authority")
	}
	next, err := cognition.NewWorldRevision(
		episode.ID, expected.Number+1, runtimeDigest(fmt.Sprintf("revision-%d", expected.Number+1)),
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	previous := expected
	transition := cognition.Transition{
		ActionID: action.ID, Previous: &previous, Current: next, Cost: 1, Terminal: h.nextTerminal,
	}
	if transition.Terminal {
		transition.PublicOutcome = "The registered goal state was reached."
	}
	if err := transition.ValidateApply(episode, expected, action); err != nil {
		return cognition.Transition{}, err
	}
	h.receipts[action.ID] = transition.Clone()
	h.env = next
	return transition.Clone(), nil
}

func (h *runtimeHarness) Start(context.Context, cognition.ScenarioRef) (cognition.Transition, error) {
	return cognition.Transition{}, errors.New("runtime tests begin from a durable episode")
}

func (h *runtimeHarness) RecordFailure(
	_ context.Context,
	command FailureMutation,
) (ActionRecord, error) {
	h.order = append(h.order, "failure")
	if h.unresolved == nil || h.unresolved.Status != ActionDispatched || h.unresolved.Action.ID != command.ActionID {
		return ActionRecord{}, errors.New("failure has no dispatched action")
	}
	failure := command.Failure.Clone()
	h.unresolved.Status, h.unresolved.Failure = ActionFailed, &failure
	record := h.unresolved.Clone()
	h.unresolved = nil
	return record, nil
}

func (h *runtimeHarness) RecordTransition(
	_ context.Context,
	command TransitionMutation,
) (ActionRecord, error) {
	h.order = append(h.order, "transition")
	if h.unresolved == nil || h.unresolved.Status != ActionDispatched || h.unresolved.Action.ID != command.ActionID {
		return ActionRecord{}, errors.New("transition has no dispatched action")
	}
	if h.transitionWriteFailures > 0 {
		h.transitionWriteFailures--
		return ActionRecord{}, errors.New("injected transition write failure")
	}
	h.unresolved.Status = ActionSucceeded
	if h.corruptResolvedAction {
		h.unresolved.Decision.ExpectedEffect = "Changed after environment execution."
	}
	result := command.Transition.Current
	h.unresolved.ResultRevision = &result
	record := h.unresolved.Clone()
	h.unresolved = nil
	h.journal = result
	h.terminal, h.public = command.Transition.Terminal, command.Transition.PublicOutcome
	return record, nil
}

func (h *runtimeHarness) AdvanceSatisfied(
	_ context.Context,
	command CompletionCommand,
) (EpisodeProgress, error) {
	h.order = append(h.order, "advance")
	graph, err := cognition.RestoreObligationGraph(h.graph)
	if err != nil {
		return EpisodeProgress{}, err
	}
	current, exists := graph.Obligation(command.Result.ObligationID)
	if !exists {
		return EpisodeProgress{}, errors.New("completion obligation is missing")
	}
	known := make(map[cognition.EvidenceRef]struct{}, len(current.SupportingRefs))
	for _, ref := range current.SupportingRefs {
		known[ref] = struct{}{}
	}
	missing := make([]cognition.EvidenceRef, 0, len(command.Result.EvidenceRefs))
	for _, ref := range command.Result.EvidenceRefs {
		if _, present := known[ref]; !present {
			missing = append(missing, ref)
		}
	}
	if len(missing) > 0 {
		if err := graph.AddSupportingEvidence(command.Result.ObligationID, h.graph.Generation, missing); err != nil {
			return EpisodeProgress{}, err
		}
	}
	if err := graph.Satisfy(command.Result.ObligationID, h.graph.Generation, command.Result); err != nil {
		return EpisodeProgress{}, err
	}
	h.graph, h.version = graph.Snapshot(), h.version+1
	status, err := graph.TerminalStatus()
	if err != nil {
		return EpisodeProgress{}, err
	}
	if status == cognition.ObligationGraphRunning {
		for _, obligation := range graph.Snapshot().Obligations {
			if obligation.Status != cognition.ObligationReady {
				continue
			}
			if err := graph.Transition(obligation.ID, h.graph.Generation, cognition.ObligationActive); err != nil {
				return EpisodeProgress{}, err
			}
			h.graph = graph.Snapshot()
			return EpisodeProgress{
				Episode: command.Binding.Episode, State: ProgressActive, Revision: command.Result.Revision,
				GraphVersion: h.version, ObligationGraph: h.graph.Clone(),
			}, nil
		}
		return EpisodeProgress{}, errors.New("test graph has no ready continuation")
	}
	if status != cognition.ObligationGraphSatisfied {
		return EpisodeProgress{}, errors.New("test graph failed while advancing satisfaction")
	}
	completion := command.Result.Clone()
	progress := EpisodeProgress{
		Episode: command.Binding.Episode, State: ProgressCompleted, Revision: command.Result.Revision,
		GraphVersion: h.version, ObligationGraph: h.graph.Clone(), Completion: &completion,
		PublicOutcome: command.PublicOutcome,
	}
	h.terminalProgress = &progress
	return cloneEpisodeProgress(progress), nil
}

func (h *runtimeHarness) FailTerminal(
	_ context.Context,
	command CompletionCommand,
) (EpisodeProgress, error) {
	h.order = append(h.order, "fail-terminal")
	graph, err := cognition.RestoreObligationGraph(h.graph)
	if err != nil {
		return EpisodeProgress{}, err
	}
	if err := graph.Transition(
		command.Result.ObligationID, h.graph.Generation, cognition.ObligationFailed,
	); err != nil {
		return EpisodeProgress{}, err
	}
	h.graph, h.version = graph.Snapshot(), h.version+1
	completion := command.Result.Clone()
	progress := EpisodeProgress{
		Episode: command.Binding.Episode, State: ProgressFailed, Revision: command.Result.Revision,
		GraphVersion: h.version, ObligationGraph: h.graph.Clone(), Completion: &completion,
		PublicOutcome: command.PublicOutcome,
	}
	h.terminalProgress = &progress
	return cloneEpisodeProgress(progress), nil
}

func (h *runtimeHarness) Seal(_ context.Context, command SealCommand) (TerminalSeal, error) {
	h.order = append(h.order, "seal")
	if h.sealFailures > 0 {
		h.sealFailures--
		return TerminalSeal{}, errors.New("injected terminal seal failure")
	}
	return TerminalSeal{
		Episode: command.Binding.Episode, Outcome: command.Outcome,
		Revision: command.Revision, TraceSHA256: runtimeDigest("sealed-trace"),
	}, nil
}

func cloneEpisodeProgress(progress EpisodeProgress) EpisodeProgress {
	progress.ObligationGraph = progress.ObligationGraph.Clone()
	if progress.Completion != nil {
		completion := progress.Completion.Clone()
		progress.Completion = &completion
	}
	if progress.Cancellation != nil {
		cancellation := *progress.Cancellation
		progress.Cancellation = &cancellation
	}
	return progress
}

func sameAppliedAction(left, right cognition.RegisteredAction) bool {
	left.Actor, right.Actor = cognition.AttemptRef{}, cognition.AttemptRef{}
	return reflect.DeepEqual(left, right)
}
