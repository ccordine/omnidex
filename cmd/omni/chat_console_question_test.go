package main

import (
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestQuestionChangeCannotRetargetBytesAlreadyRead(t *testing.T) {
	source := newBlockingPlanReviewSource()
	defer source.Close()
	var released sync.Once
	release := func() { released.Do(func() { close(source.release) }) }
	defer release()
	router, err := newPlanReviewInputRouter(source)
	if err != nil {
		t.Fatal(err)
	}
	console := &chatConsole{reviewInput: router}
	pause := model.CodingAgentPause{Generation: 2, StepID: 11, WaitingAttempt: 3}
	console.SetQuestionAuthority(41, &pause)
	original := router.CurrentInputAuthority()
	type readResult struct {
		text      string
		authority terminalInputAuthority
		err       error
	}
	read := make(chan readResult, 1)
	source.pauseNext.Store(true)
	go func() {
		buffer := make([]byte, 128)
		count, authority, err := router.ReadWithAuthority(buffer)
		read <- readResult{string(buffer[:count]), authority, err}
	}()
	source.values <- []byte("answer typed for original question\r")
	select {
	case <-source.entered:
	case <-time.After(time.Second):
		t.Fatal("source did not enter controlled in-flight read")
	}
	pause.WaitingAttempt = 4
	changed := make(chan struct{})
	go func() { console.SetQuestionAuthority(41, &pause); close(changed) }()
	select {
	case <-changed:
		t.Fatal("question changed before original question bytes were routed")
	case <-time.After(2 * planReviewEscapeTimeout):
	}
	release()
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("question change did not finish after source read")
	}
	select {
	case result := <-read:
		if result.err != nil || result.text != "answer typed for original question\r" || result.authority != original {
			t.Fatalf("original bytes lost question authority: text=%q authority=%#v error=%v", result.text, result.authority, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("original bytes were not delivered")
	}
	if router.CurrentInputAuthority() == original {
		t.Fatal("new question retained old input authority")
	}
}
