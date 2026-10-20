package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func (session *chatSession) beginPlanReviewNote(leafID model.CodingPlanLeafID) error {
	if session.planReview == nil {
		return fmt.Errorf("cannot replan without active plan-review authority")
	}
	subject, err := planReviewNoteSubject(session.planReview.snapshot, leafID)
	if err != nil {
		return err
	}
	jobID := session.planReview.snapshot.JobID
	authority, err := session.renderer.console.BeginPlanReviewNote()
	if err != nil {
		return err
	}
	session.planReviewInputs = nil
	session.planNoteEditing = true
	session.planNoteAuthority = authority
	session.planNoteSubject = subject
	if err := session.renderer.console.SetPrompt(fmt.Sprintf("plan #%d> ", jobID)); err != nil {
		return err
	}
	return session.renderer.system(
		"job %d · reply naturally to discuss, change, approve or start the plan; empty line opens ledger controls",
		jobID,
	)
}

func (session *chatSession) acceptPlanReviewNote(note string) error {
	if !session.planNoteEditing || session.planReview == nil {
		return fmt.Errorf("plan-review note editor lacks active review authority")
	}
	if strings.TrimSpace(note) == "" {
		if err := session.endPlanReviewNoteInput(); err != nil {
			return err
		}
		session.planNoteEditing = false
		session.planNoteSubject = ""
		return session.showPlanReview()
	}
	return session.acceptUserMessage(note)
}

func (session *chatSession) endPlanReviewNoteInput() error {
	authority := session.planNoteAuthority
	if err := session.renderer.console.EndPlanReviewNote(authority); err != nil {
		return err
	}
	session.planNoteAuthority = terminalInputAuthority{}
	return nil
}

// Typed lifecycle controls remain controls while the planning editor is open.
// Suppress review re-entry during the operation's authoritative snapshot reads.
func (session *chatSession) acceptPlanReviewControl(line string) (bool, error) {
	if err := session.endPlanReviewNoteInput(); err != nil {
		return false, err
	}
	session.planNoteEditing = false
	session.planNoteSubject = ""
	session.planNoteSubmitting = true
	quit, err := session.acceptInput(line, false)
	session.planNoteSubmitting = false
	if !quit {
		err = errors.Join(err, session.reconcilePlanReview())
	}
	return quit, err
}

func planReviewNoteSubject(
	plan model.CodingPlan,
	leafID model.CodingPlanLeafID,
) (string, error) {
	if leafID == "" {
		return "", nil
	}
	if _, err := model.ParseCodingPlanLeafID(string(leafID)); err != nil {
		return "", fmt.Errorf("selected planning leaf: %w", err)
	}
	for _, leaf := range plan.Leaves {
		if leaf.ID == leafID {
			return leaf.Statement, nil
		}
	}
	return "", fmt.Errorf("selected planning leaf is absent from the authoritative review")
}
