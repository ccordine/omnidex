package cognitiongauntlet

import (
	"context"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type liveStaleCallJournal struct {
	probe *liveStalePortController
	base  cognitionpolicy.CallJournal
}

type liveStaleReconciler struct {
	probe *liveStalePortController
	base  cognitionruntime.DecisionReconciler
}

type liveStaleEnvironment struct {
	probe *liveStalePortController
	base  cognition.Environment
}

type liveStaleEpisodeJournal struct {
	probe *liveStalePortController
	base  cognitionruntime.EpisodeJournal
}

func (journal liveStaleCallJournal) Start(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	return journal.base.Start(ctx, attempt)
}

func (journal liveStaleCallJournal) Finish(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	command := struct {
		Attempt  cognitionpolicy.CallAttempt  `json:"attempt"`
		Result   cognitionpolicy.CallResult   `json:"result"`
		Evidence cognitionpolicy.CallEvidence `json:"evidence"`
	}{attempt, result, evidence}
	paused, err := journal.probe.before(liveStalePolicyFinish, command)
	if err != nil {
		return err
	}
	err = journal.base.Finish(ctx, attempt, result, evidence)
	if paused {
		return journal.probe.afterPolicy(command, result, err)
	}
	return err
}

func (reconciler liveStaleReconciler) Reconcile(
	ctx context.Context,
	command cognitionruntime.ReconciliationCommand,
) (cognitionruntime.ReconciliationReceipt, error) {
	paused, err := reconciler.probe.before(liveStaleReconcile, command)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	receipt, err := reconciler.base.Reconcile(ctx, command)
	if paused {
		err = reconciler.probe.after(liveStaleReconcile, command, err)
	}
	return receipt, err
}

func (environment liveStaleEnvironment) Start(
	ctx context.Context,
	scenario cognition.ScenarioRef,
) (cognition.Transition, error) {
	return environment.base.Start(ctx, scenario)
}

func (environment liveStaleEnvironment) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	command := struct {
		Episode  cognition.EpisodeRef       `json:"episode"`
		Expected cognition.WorldRevision    `json:"expected"`
		Action   cognition.RegisteredAction `json:"action"`
	}{episode, expected, action}
	paused, err := environment.probe.before(liveStaleEnvironmentApply, command)
	if err != nil {
		return cognition.Transition{}, err
	}
	transition, err := environment.base.Apply(ctx, episode, expected, action)
	if paused {
		err = environment.probe.after(liveStaleEnvironmentApply, command, err)
	}
	return transition, err
}

func (episodes liveStaleEpisodeJournal) TerminalProgress(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.EpisodeProgress, error) {
	return episodes.base.TerminalProgress(ctx, binding)
}

func (episodes liveStaleEpisodeJournal) AdvanceSatisfied(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	return episodes.terminal(ctx, command, true)
}

func (episodes liveStaleEpisodeJournal) FailTerminal(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	return episodes.terminal(ctx, command, false)
}

func (episodes liveStaleEpisodeJournal) terminal(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
	satisfied bool,
) (cognitionruntime.EpisodeProgress, error) {
	paused, err := episodes.probe.before(liveStaleTerminal, command)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	var progress cognitionruntime.EpisodeProgress
	if satisfied {
		progress, err = episodes.base.AdvanceSatisfied(ctx, command)
	} else {
		progress, err = episodes.base.FailTerminal(ctx, command)
	}
	if paused {
		err = episodes.probe.after(liveStaleTerminal, command, err)
	}
	return progress, err
}

var (
	_ cognitionpolicy.CallJournal         = liveStaleCallJournal{}
	_ cognitionruntime.DecisionReconciler = liveStaleReconciler{}
	_ cognition.Environment               = liveStaleEnvironment{}
	_ cognitionruntime.EpisodeJournal     = liveStaleEpisodeJournal{}
)
