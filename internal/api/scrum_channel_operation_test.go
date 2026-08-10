package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

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
	update, err := buildScrumChannelCardUpdate(card, request, "started", []string{"planner"}, job)
	if err != nil {
		t.Fatal(err)
	}
	var messages []ScrumChatMessage
	if err := json.Unmarshal(update.Chat, &messages); err != nil {
		t.Fatal(err)
	}
	bound := 0
	for _, message := range messages {
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
	if !strings.Contains(update.ConsoleLog, "Job #42 queued from channel") || !strings.Contains(update.ConsoleLog, "Models: planner") {
		t.Fatalf("console log=%q", update.ConsoleLog)
	}
}

func TestBuildScrumChannelCardUpdateRejectsOrphanedOperationMessage(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("api-scrum-channel-orphan")
	if err != nil {
		t.Fatal(err)
	}
	card := scrumChannelBuilderCard()
	card.Chat = json.RawMessage(`[{"id":"existing","role":"user","content":"Earlier","created_at":"2026-08-09T00:00:00Z","operation_id":"` + string(operationID) + `"}]`)
	request := queue.ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: card.ProjectID, CardID: card.ID, Message: "Continue.",
	}
	if _, err := buildScrumChannelCardUpdate(card, request, "started", nil, model.Job{ID: 42}); err == nil {
		t.Fatal("operation-bound message without immutable result must fail")
	}
}

func TestScrumChannelResultNoteRejectsUnknownAction(t *testing.T) {
	if _, err := scrumChannelResultNote("unknown", 42); err == nil {
		t.Fatal("unknown Scrum channel result action must fail loudly")
	}
}

func scrumChannelBuilderCard() queue.DBScrumCard {
	now := time.Now().UTC()
	return queue.DBScrumCard{
		ID: "card-operation", ProjectID: 7, Title: "Operation", Column: "assigned",
		Checklist: json.RawMessage(`[]`), RefFiles: json.RawMessage(`[]`), Chat: json.RawMessage(`[]`),
		ModelConfig: json.RawMessage(`{}`), AgentConfig: json.RawMessage(`{}`), Recipe: json.RawMessage(`{}`),
		Tags: json.RawMessage(`[]`), PlanningChat: json.RawMessage(`[]`), CoachConfig: json.RawMessage(`{}`),
		TestCriteria: json.RawMessage(`[]`), FlowMetrics: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
	}
}
