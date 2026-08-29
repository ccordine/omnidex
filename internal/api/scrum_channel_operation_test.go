package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestWriteScrumChannelDispatchResponseAttestsExactOperationIdentity(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("api-scrum-channel-response")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	writeScrumChannelDispatchResponse(recorder, scrumChannelDispatchResult{
		OperationID: operationID,
		ProjectID:   7,
		Card:        ScrumCard{ID: "card-operation"},
		Action:      "replanned",
	})
	var response map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response) != 4 || string(response["operation_id"]) != `"`+string(operationID)+`"` ||
		string(response["project_id"]) != "7" || string(response["action"]) != `"replanned"` {
		t.Fatalf("channel response identity=%s", recorder.Body.String())
	}
}

func TestBuildScrumChannelCardUpdateBindsOneUserMessageToOperation(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("api-scrum-channel-builder")
	if err != nil {
		t.Fatal(err)
	}
	card := scrumChannelBuilderCard()
	request := queue.ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: card.ProjectID, CardID: card.ID,
		Message: "Continue with the accepted correction.",
	}
	job := model.Job{ID: 42}
	update, err := buildScrumChannelCardUpdate(card, request, "started", job)
	if err != nil {
		t.Fatal(err)
	}
	bound := 0
	for _, message := range update.Messages {
		if message.OperationID == string(operationID) {
			bound++
			if message.Role != "user" || message.Content != request.Message {
				t.Fatalf("operation message=%+v", message)
			}
		}
	}
	if bound != 1 || update.JobID != "42" || update.Column != "in_progress" || update.PlayState != scrumPlayRunning {
		t.Fatalf("bound=%d update=%+v", bound, update)
	}
	noteFound := false
	for _, message := range update.Messages {
		if message.Role == "system" && strings.Contains(message.Content, "Job #42 queued from channel") {
			noteFound = true
		}
	}
	if !noteFound {
		t.Fatalf("typed system note missing from update: %+v", update.Messages)
	}
}

func TestScrumChannelResultNoteRejectsUnknownAction(t *testing.T) {
	if _, err := scrumChannelResultNote("unknown", 42); err == nil {
		t.Fatal("unknown Scrum channel result action must fail loudly")
	}
}

func TestScrumChannelResultNoteUsesJobAuthorityVocabulary(t *testing.T) {
	note, err := scrumChannelResultNote("feedback", 42)
	if err != nil {
		t.Fatal(err)
	}
	if note != "Channel message sent to waiting job" || strings.Contains(note, "agent") {
		t.Fatalf("feedback note=%q", note)
	}
}

func scrumChannelBuilderCard() queue.DBScrumCard {
	now := time.Now().UTC()
	return queue.DBScrumCard{
		ID: "card-operation", ProjectID: 7, Title: "Operation", Column: "assigned",
		Checklist: json.RawMessage(`[]`), RefFiles: json.RawMessage(`[]`),
		Tags:         json.RawMessage(`[]`),
		TestCriteria: json.RawMessage(`[]`), FlowMetrics: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
	}
}
