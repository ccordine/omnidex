package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestInterruptedPlanningWaitDoesNotRequestAnUnfinishedReview(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "no proposed revision exists yet", 409)
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	session.active = waitingPlanReviewJob(t, session, 73, 4)
	session.controls["interrupt-op"] = model.ChannelSessionControl{Kind: model.ChannelSessionControlInterrupt, JobID: 73, Generation: 4}
	if err := session.reconcilePlanReview(); err != nil || requests != 0 || session.planReview != nil {
		t.Fatalf("interrupted planning tried to review the previous frozen ledger: requests=%d error=%v", requests, err)
	}
	for _, control := range []model.ChannelSessionControl{
		{Kind: model.ChannelSessionControlInterrupt, JobID: 72, Generation: 4},
		{Kind: model.ChannelSessionControlInterrupt, JobID: 73, Generation: 3},
		{Kind: model.ChannelSessionControlReplan, JobID: 73, Generation: 4},
	} {
		session.controls["interrupt-op"] = control
		if session.currentJobInterrupted() {
			t.Fatalf("foreign or obsolete control suppressed review: %+v", control)
		}
	}
}
