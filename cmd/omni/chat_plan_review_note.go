package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
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
	if err := session.renderer.console.SetPrompt(fmt.Sprintf("plan note #%d> ", jobID)); err != nil {
		return err
	}
	return session.renderer.system(
		"job %d · enter one exact planning note to replan this same job; submit an empty line to return",
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
		if err := session.renderer.system("plan note canceled"); err != nil {
			return err
		}
		return session.showPlanReview()
	}
	feedback, err := planReviewNoteFeedback(session.planNoteSubject, note)
	if err != nil {
		return err
	}
	jobID := session.planReview.snapshot.JobID
	session.planNoteEditing = false
	session.planNoteSubmitting = true
	_, err = session.control("redirect", feedback, true, &jobID)
	session.planNoteSubmitting = false
	if err != nil {
		if errors.Is(err, errChatRequestInterrupted) {
			if endErr := session.endPlanReviewNoteInput(); endErr != nil {
				return errors.Join(err, endErr)
			}
			return err
		}
		if definitiveChatRequestFailure(err) {
			if endErr := session.endPlanReviewNoteInput(); endErr != nil {
				return errors.Join(err, endErr)
			}
			session.planNoteSubject = ""
			if showErr := session.showPlanReview(); showErr != nil {
				return errors.Join(err, showErr)
			}
			return err
		}
		session.planNoteEditing = true
		if promptErr := session.renderer.console.SetPrompt(fmt.Sprintf("plan note #%d> ", jobID)); promptErr != nil {
			return errors.Join(err, promptErr)
		}
		return err
	}
	if err := session.endPlanReviewNoteInput(); err != nil {
		return err
	}
	session.planNoteSubject = ""
	session.planReview = nil
	session.planReviewInputs = nil
	return session.reloadSnapshot()
}

func (session *chatSession) endPlanReviewNoteInput() error {
	authority := session.planNoteAuthority
	if err := session.renderer.console.EndPlanReviewNote(authority); err != nil {
		return err
	}
	session.planNoteAuthority = terminalInputAuthority{}
	return nil
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

func planReviewNoteFeedback(subject, note string) (string, error) {
	feedback := note
	if subject != "" {
		feedback = "The user's planning note concerns this proposed outcome:\n\n" +
			subject + "\n\n" + note
	}
	if err := client.ValidateReplanFeedback(feedback); err != nil {
		return "", err
	}
	return feedback, nil
}
