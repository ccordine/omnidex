package main

import "github.com/gryph/omnidex/internal/model"

type chatQuestionScope struct {
	jobID, generation, stepID, attempt int64
}

// Input belongs to the question that was visible when typing began. A new
// question invalidates that authority without discarding the user's text.
func (console *chatConsole) SetQuestionAuthority(jobID int64, pause *model.CodingAgentPause) {
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	scope := chatQuestionScope{}
	if pause != nil {
		scope = chatQuestionScope{jobID, pause.Generation, pause.StepID, pause.WaitingAttempt}
	}
	if scope == console.questionScope {
		return
	}
	console.questionScope = scope
	if console.reviewInput != nil {
		console.reviewInput.source.LockInput()
		defer console.reviewInput.source.UnlockInput()
		console.reviewInput.mu.Lock()
		console.reviewInput.modeGeneration++
		console.reviewInput.signalReadLocked()
		console.reviewInput.mu.Unlock()
	}
}
