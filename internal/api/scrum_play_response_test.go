package api

import "testing"

func TestScrumPlayResponseBindsProjectCardActionAndJob(t *testing.T) {
	running := ScrumCard{ID: "card-7", Column: "in_progress", PlayState: scrumPlayRunning, JobID: "41"}
	response, err := newScrumCardPlayResponse(14, "card-7", "started", running)
	if err != nil {
		t.Fatal(err)
	}
	if response.ProjectID != 14 || response.CardID != "card-7" || response.Action != scrumCardPlayStarted ||
		response.JobID != "41" || response.QueueOrder != 0 || response.Card.ID != "card-7" {
		t.Fatalf("response=%+v", response)
	}

	queued := ScrumCard{ID: "card-7", Column: "assigned", PlayState: scrumPlayQueued, QueueOrder: 3}
	response, err = newScrumCardPlayResponse(14, "card-7", "queued", queued)
	if err != nil || response.JobID != "" || response.QueueOrder != 3 {
		t.Fatalf("response=%+v error=%v", response, err)
	}
}

func TestScrumPlayResponseRejectsContradictoryOrUnregisteredAuthority(t *testing.T) {
	for name, test := range map[string]struct {
		projectID int64
		cardID    string
		action    string
		card      ScrumCard
	}{
		"wrong project":  {0, "card-7", "started", ScrumCard{ID: "card-7", Column: "in_progress", PlayState: scrumPlayRunning, JobID: "41"}},
		"wrong card":     {14, "card-7", "started", ScrumCard{ID: "other", Column: "in_progress", PlayState: scrumPlayRunning, JobID: "41"}},
		"missing job":    {14, "card-7", "started", ScrumCard{ID: "card-7", Column: "in_progress", PlayState: scrumPlayRunning}},
		"queued job":     {14, "card-7", "queued", ScrumCard{ID: "card-7", Column: "assigned", PlayState: scrumPlayQueued, JobID: "41", QueueOrder: 1}},
		"unknown action": {14, "card-7", "agent", ScrumCard{ID: "card-7", Column: "in_progress", PlayState: scrumPlayRunning, JobID: "41"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newScrumCardPlayResponse(test.projectID, test.cardID, test.action, test.card); err == nil {
				t.Fatal("contradictory response authority was accepted")
			}
		})
	}
}

func TestScrumPauseResponseRequiresExactPausedPostState(t *testing.T) {
	card := ScrumCard{ID: "card-7", Column: "assigned", PlayState: scrumPlayPaused}
	response, err := newScrumCardPauseResponse(14, "card-7", card)
	if err != nil || response.Action != scrumCardPlayPaused || response.ProjectID != 14 {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	card.JobID = "41"
	if _, err := newScrumCardPauseResponse(14, "card-7", card); err == nil {
		t.Fatal("paused response with a live job was accepted")
	}
}
