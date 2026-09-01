package main

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

const codingPlanReviewStepAction = "v3_coding_plan"

type planReviewInput struct {
	Key planReviewKey
	Err error
	EOF bool
}

func readPlanReviewInput(
	ctx context.Context,
	router *planReviewInputRouter,
) <-chan planReviewInput {
	inputs := make(chan planReviewInput, 8)
	go func() {
		defer close(inputs)
		if router == nil {
			sendPlanReviewInput(ctx, inputs, planReviewInput{
				Err: fmt.Errorf("plan review input router is unavailable"),
			})
			return
		}
		for {
			key, err := router.NextReviewKey(ctx)
			if err != nil {
				sendPlanReviewInput(ctx, inputs, planReviewInput{Err: err})
				return
			}
			if key == planReviewKeyEOF {
				sendPlanReviewInput(ctx, inputs, planReviewInput{EOF: true})
				return
			}
			if !sendPlanReviewInput(ctx, inputs, planReviewInput{Key: key}) {
				return
			}
		}
	}()
	return inputs
}

func sendPlanReviewInput(
	ctx context.Context,
	destination chan<- planReviewInput,
	input planReviewInput,
) bool {
	select {
	case <-ctx.Done():
		return false
	case destination <- input:
		return true
	}
}

func (session *chatSession) reconcilePlanReview() error {
	if session.planNoteEditing || session.planNoteSubmitting {
		return nil
	}
	var staleObservation error
	for attempt := 0; attempt < 2; attempt++ {
		required, err := activeJobRequiresPlanReview(session.active)
		if err != nil {
			return err
		}
		if !required {
			return session.clearPlanReview()
		}
		jobID := session.active.Job.ID
		generation := session.active.Job.CurrentGeneration
		plan, err := awaitChatRequest(
			session.ctx,
			session.signals,
			func(requestContext context.Context) (model.CodingPlan, error) {
				return session.client.CodingPlan(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					jobID,
				)
			},
		)
		switch {
		case err != nil:
			staleObservation = fmt.Errorf(
				"load required coding plan review for job %d: %w", jobID, err,
			)
		case plan.Generation != generation:
			staleObservation = fmt.Errorf(
				"coding plan generation %d differs from active job generation %d",
				plan.Generation,
				generation,
			)
		case plan.State != model.CodingPlanStateReview:
			staleObservation = fmt.Errorf(
				"coding plan state %q differs from active review state", plan.State,
			)
		default:
			if session.planReview == nil {
				state, err := newPlanReviewState(plan)
				if err != nil {
					return err
				}
				session.planReview = &state
			} else {
				state, _, err := reconcilePlanReviewState(*session.planReview, plan)
				if err != nil {
					return err
				}
				session.planReview = &state
			}
			return session.showPlanReview()
		}
		if attempt == 1 {
			return staleObservation
		}
		if err := session.reloadSnapshotAuthority(); err != nil {
			return fmt.Errorf("%v; reload authoritative CLI session: %w", staleObservation, err)
		}
	}
	return staleObservation
}

func activeJobRequiresPlanReview(details *model.JobDetails) (bool, error) {
	if details == nil {
		return false, nil
	}
	waitingPlanSteps := 0
	for _, step := range details.Steps {
		if step.Generation != details.Job.CurrentGeneration || step.SupersededAtGeneration != nil ||
			step.Action != codingPlanReviewStepAction || step.Status != model.StepStatusWaiting {
			continue
		}
		waitingPlanSteps++
	}
	if waitingPlanSteps > 1 {
		return false, fmt.Errorf(
			"job %d has %d current coding-plan review steps",
			details.Job.ID,
			waitingPlanSteps,
		)
	}
	if waitingPlanSteps == 0 {
		return false, nil
	}
	if details.Job.Status != model.JobStatusWaiting {
		return false, fmt.Errorf(
			"job %d has a waiting coding-plan review step while job status is %q",
			details.Job.ID,
			details.Job.Status,
		)
	}
	return true, nil
}

func (session *chatSession) showPlanReview() error {
	if session.planReview == nil {
		return fmt.Errorf("plan review state is unavailable")
	}
	view, err := renderPlanReview(*session.planReview)
	if err != nil {
		return err
	}
	console := session.renderer.console
	if err := console.ShowPlanReview(view); err != nil {
		return err
	}
	if session.planReviewInputs == nil {
		session.planReviewInputs = readPlanReviewInput(session.ctx, console.PlanReviewInput())
	}
	return nil
}

func (session *chatSession) clearPlanReview() error {
	if session.planReview == nil && session.planReviewInputs == nil {
		return nil
	}
	if err := session.renderer.console.HidePlanReview(); err != nil {
		return err
	}
	session.planReview = nil
	session.planReviewInputs = nil
	return nil
}

func (session *chatSession) acceptPlanReviewKey(key planReviewKey) error {
	if session.planReview == nil {
		return fmt.Errorf("received plan-review input without an active review")
	}
	action, err := planReviewActionForKey(key)
	if err != nil {
		return err
	}
	next, effect, err := reducePlanReviewState(*session.planReview, action)
	if err != nil {
		return err
	}
	session.planReview = &next
	switch effect.Kind {
	case "":
		return session.showPlanReview()
	case planReviewEffectDecisionRequested:
		return session.persistPlanReviewDecision(effect.LeafID, effect.Decision)
	case planReviewEffectFreezeRequested:
		return session.freezePlanReview()
	case planReviewEffectNoteRequested:
		return session.beginPlanReviewNote(effect.LeafID)
	default:
		return fmt.Errorf("plan review produced unsupported effect %q", effect.Kind)
	}
}

func planReviewActionForKey(key planReviewKey) (planReviewAction, error) {
	switch key {
	case planReviewKeyUp:
		return planReviewActionUp, nil
	case planReviewKeyDown:
		return planReviewActionDown, nil
	case planReviewKeyToggle:
		return planReviewActionToggle, nil
	case planReviewKeyEnter:
		return planReviewActionEnter, nil
	case planReviewKeyEscape:
		return planReviewActionCancel, nil
	case planReviewKeyNote:
		return planReviewActionRequestNote, nil
	default:
		return "", fmt.Errorf("plan review key %d is unsupported", key)
	}
}
