package main

import (
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
)

const (
	planReviewActionUp          planReviewAction = "up"
	planReviewActionDown        planReviewAction = "down"
	planReviewActionToggle      planReviewAction = "toggle"
	planReviewActionEnter       planReviewAction = "enter"
	planReviewActionCancel      planReviewAction = "cancel"
	planReviewActionRequestNote planReviewAction = "request_note"

	planReviewEffectDecisionRequested planReviewEffectKind = "decision_requested"
	planReviewEffectFreezeRequested   planReviewEffectKind = "freeze_requested"
	planReviewEffectNoteRequested     planReviewEffectKind = "note_requested"

	planReviewSnapshotUnchanged       planReviewSnapshotChange = "unchanged"
	planReviewSnapshotRevisionChanged planReviewSnapshotChange = "revision_changed"
	planReviewSnapshotIdentityChanged planReviewSnapshotChange = "identity_changed"
)

type planReviewAction string
type planReviewEffectKind string
type planReviewSnapshotChange string

type planReviewState struct {
	snapshot   model.CodingPlan
	selected   int
	confirming bool
}

// planReviewEffect describes an operation for the imperative caller. A toggle
// deliberately leaves state unchanged until the server returns the persisted
// decision in a new snapshot.
type planReviewEffect struct {
	Kind     planReviewEffectKind
	LeafID   model.CodingPlanLeafID
	Decision model.CodingPlanDecision
}

func newPlanReviewState(snapshot model.CodingPlan) (planReviewState, error) {
	if err := validatePlanReviewSnapshot(snapshot); err != nil {
		return planReviewState{}, err
	}
	snapshot.Leaves = slices.Clone(snapshot.Leaves)
	return planReviewState{snapshot: snapshot, selected: 0}, nil
}

func reconcilePlanReviewState(
	current planReviewState,
	snapshot model.CodingPlan,
) (planReviewState, planReviewSnapshotChange, error) {
	if err := validatePlanReviewState(current); err != nil {
		return planReviewState{}, "", fmt.Errorf("retained plan review state: %w", err)
	}
	if err := validatePlanReviewSnapshot(snapshot); err != nil {
		return planReviewState{}, "", fmt.Errorf("received plan review snapshot: %w", err)
	}

	sameIdentity := current.snapshot.JobID == snapshot.JobID &&
		current.snapshot.Generation == snapshot.Generation
	if sameIdentity && current.snapshot.Revision == snapshot.Revision {
		if !equalPlanReviewSnapshots(current.snapshot, snapshot) {
			return planReviewState{}, "", fmt.Errorf(
				"plan review revision %d changed without a new revision identity",
				snapshot.Revision,
			)
		}
		return current, planReviewSnapshotUnchanged, nil
	}
	if sameIdentity && snapshot.Revision < current.snapshot.Revision {
		return planReviewState{}, "", fmt.Errorf(
			"plan review revision regressed from %d to %d",
			current.snapshot.Revision,
			snapshot.Revision,
		)
	}
	if sameIdentity {
		if err := validatePlanReviewRevisionTransition(current.snapshot, snapshot); err != nil {
			return planReviewState{}, "", err
		}
	}

	next, err := newPlanReviewState(snapshot)
	if err != nil {
		return planReviewState{}, "", err
	}
	if current.snapshot.JobID == snapshot.JobID && len(current.snapshot.Leaves) > 0 {
		selectedID := current.snapshot.Leaves[current.selected].ID
		for index := range next.snapshot.Leaves {
			if next.snapshot.Leaves[index].ID == selectedID {
				next.selected = index
				break
			}
		}
	}
	if !sameIdentity {
		return next, planReviewSnapshotIdentityChanged, nil
	}
	return next, planReviewSnapshotRevisionChanged, nil
}

func reducePlanReviewState(
	current planReviewState,
	action planReviewAction,
) (planReviewState, planReviewEffect, error) {
	if err := validatePlanReviewState(current); err != nil {
		return planReviewState{}, planReviewEffect{}, err
	}
	if !validPlanReviewAction(action) {
		return planReviewState{}, planReviewEffect{}, fmt.Errorf(
			"plan review action %q is unsupported", action,
		)
	}
	if current.snapshot.State != model.CodingPlanStateReview {
		return planReviewState{}, planReviewEffect{}, fmt.Errorf(
			"plan review snapshot is %q and cannot accept review input",
			current.snapshot.State,
		)
	}

	if current.confirming {
		switch action {
		case planReviewActionEnter:
			return current, planReviewEffect{Kind: planReviewEffectFreezeRequested}, nil
		case planReviewActionCancel:
			current.confirming = false
		case planReviewActionRequestNote:
			current.confirming = false
			return current, notePlanReviewEffect(current), nil
		}
		return current, planReviewEffect{}, nil
	}

	switch action {
	case planReviewActionUp:
		if current.selected > 0 {
			current.selected--
		}
	case planReviewActionDown:
		if current.selected+1 < len(current.snapshot.Leaves) {
			current.selected++
		}
	case planReviewActionToggle:
		if len(current.snapshot.Leaves) == 0 {
			return current, planReviewEffect{}, nil
		}
		leaf := current.snapshot.Leaves[current.selected]
		desired := model.CodingPlanDecisionApproved
		if leaf.Decision == model.CodingPlanDecisionApproved {
			desired = model.CodingPlanDecisionRejected
		}
		return current, planReviewEffect{
			Kind: planReviewEffectDecisionRequested, LeafID: leaf.ID, Decision: desired,
		}, nil
	case planReviewActionEnter:
		if planReviewCanConfirm(current.snapshot) {
			current.confirming = true
		}
	case planReviewActionCancel:
	case planReviewActionRequestNote:
		return current, notePlanReviewEffect(current), nil
	}
	return current, planReviewEffect{}, nil
}

func notePlanReviewEffect(state planReviewState) planReviewEffect {
	if len(state.snapshot.Leaves) == 0 {
		return planReviewEffect{Kind: planReviewEffectNoteRequested}
	}
	return planReviewEffect{
		Kind:   planReviewEffectNoteRequested,
		LeafID: state.snapshot.Leaves[state.selected].ID,
	}
}

func planReviewCanConfirm(snapshot model.CodingPlan) bool {
	approved := 0
	for _, leaf := range snapshot.Leaves {
		if leaf.Decision == model.CodingPlanDecisionPending {
			return false
		}
		if leaf.Decision == model.CodingPlanDecisionApproved {
			approved++
		}
	}
	return approved > 0
}

func validPlanReviewAction(action planReviewAction) bool {
	switch action {
	case planReviewActionUp, planReviewActionDown, planReviewActionToggle,
		planReviewActionEnter, planReviewActionCancel, planReviewActionRequestNote:
		return true
	default:
		return false
	}
}

func validatePlanReviewState(state planReviewState) error {
	if err := validatePlanReviewSnapshot(state.snapshot); err != nil {
		return err
	}
	if len(state.snapshot.Leaves) == 0 {
		if state.selected != 0 {
			return fmt.Errorf("empty plan review selection must remain zero, received %d", state.selected)
		}
	} else if state.selected < 0 || state.selected >= len(state.snapshot.Leaves) {
		return fmt.Errorf("plan review selection %d is outside its leaf set", state.selected)
	}
	if state.confirming && (state.snapshot.State != model.CodingPlanStateReview ||
		!planReviewCanConfirm(state.snapshot)) {
		return fmt.Errorf("plan review confirmation lacks an eligible authoritative snapshot")
	}
	return nil
}

func validatePlanReviewSnapshot(snapshot model.CodingPlan) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("invalid coding plan authority: %w", err)
	}
	if snapshot.State != model.CodingPlanStateReview {
		return fmt.Errorf("coding plan state %q is not reviewable", snapshot.State)
	}
	return nil
}

func equalPlanReviewSnapshots(left, right model.CodingPlan) bool {
	return left.JobID == right.JobID && left.Generation == right.Generation &&
		left.Revision == right.Revision && left.State == right.State &&
		left.ScopeMode == right.ScopeMode && left.RequestSHA256 == right.RequestSHA256 &&
		left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt) &&
		left.FrozenAt == nil && right.FrozenAt == nil && slices.Equal(left.Leaves, right.Leaves)
}

func validatePlanReviewRevisionTransition(previous, next model.CodingPlan) error {
	if previous.RequestSHA256 != next.RequestSHA256 {
		return fmt.Errorf("plan review request authority changed within one job generation")
	}
	if previous.ScopeMode != next.ScopeMode {
		return fmt.Errorf("plan review scope mode changed within one job generation")
	}
	if !previous.CreatedAt.Equal(next.CreatedAt) {
		return fmt.Errorf("plan review creation authority changed within one job generation")
	}
	if next.UpdatedAt.Before(previous.UpdatedAt) {
		return fmt.Errorf("plan review update time regressed within one job generation")
	}
	if len(previous.Leaves) != len(next.Leaves) {
		return fmt.Errorf("plan review leaf set changed within one job generation")
	}
	for index := range previous.Leaves {
		left := previous.Leaves[index]
		right := next.Leaves[index]
		if left.ID != right.ID || left.Statement != right.Statement ||
			left.Annotation != right.Annotation {
			return fmt.Errorf(
				"plan review leaf %d authority changed within one job generation",
				index,
			)
		}
	}
	return nil
}
