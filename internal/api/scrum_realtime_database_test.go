package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresScrumRealtimeCardPayloadIsBoundedAndTyped(t *testing.T) {
	pool := openIsolatedAPIDatabasePool(t)
	repository := queue.New(pool)
	if err := repository.ResetDatabase(t.Context(), loadAPITestDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-realtime-%d", time.Now().UnixNano()), t.TempDir(), "")

	if err != nil {
		t.Fatal(err)
	}
	stored, err := repository.CreateScrumCard(
		t.Context(), project.ID, "card_bounded", "Typed realtime", "", "assigned", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	chat := make([]ScrumChatMessage, 0, scrumRealtimeChannelPageSize+10)
	for index := 0; index < scrumRealtimeChannelPageSize+10; index++ {
		chat = append(chat, ScrumChatMessage{
			ID: fmt.Sprintf("message_%d", index), Role: "assistant", Content: fmt.Sprintf("message %d", index),
			CreatedAt: time.Date(2026, 8, 13, 12, 0, 0, index*1_000, time.UTC).Format(time.RFC3339Nano),
		})
	}
	operationID, err := queue.NewLifecycleOperationID("scrum-realtime-page", fmt.Sprintf("%d", project.ID))
	if err != nil {
		t.Fatal(err)
	}
	request := queue.ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: project.ID, CardID: stored.ID,
		Message: "append realtime fixture rows",
	}
	chat[0].Role = "user"
	chat[0].Content = request.Message
	chat[0].OperationID = string(request.OperationID)
	messages, err := scrumChannelMessageAppends(chat)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.ExecuteScrumChannelOperation(
		t.Context(), queue.ScrumChannelOperationCommand{
			Request: request, ExpectedCardUpdatedAt: stored.UpdatedAt,
			Effect:       queue.ScrumChannelEffect{Kind: queue.ScrumChannelStartJob, Instruction: request.Message},
			ResultAction: "started",
		}, func(current queue.DBScrumCard, job model.Job) (queue.ScrumChannelCardUpdate, error) {
			return queue.ScrumChannelCardUpdate{
				Column: "in_progress", JobID: fmt.Sprintf("%d", job.ID), PlayState: "running",
				SyncJobID: fmt.Sprintf("%d", job.ID), Messages: messages,
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	stored = result.Card
	card, err := dbScrumCardToAPI(stored)
	if err != nil {
		t.Fatal(err)
	}
	card.Title = "stale caller title"
	server := &Server{repo: repository, realtimeHub: NewRealtimeHub()}
	subscription, err := server.realtimeHub.Subscribe([]string{"scrum"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()
	server.publishScrumCardUpdate(context.Background(), project.ID, card, string(scrumCardRealtimeJobProgress))

	select {
	case raw := <-subscription.Messages:
		if strings.Contains(string(raw), `"html"`) || strings.Contains(string(raw), "data-recyclr") {
			t.Fatalf("typed realtime must not contain HTML: %s", raw)
		}
		var message realtimeMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		if message.Card == nil || len(message.Card.Chat) != scrumRealtimeChannelPageSize {
			t.Fatalf("unexpected bounded card: %+v", message.Card)
		}
		if message.Card.Title != "Typed realtime" {
			t.Fatalf("realtime used stale caller state: %+v", message.Card)
		}
		if message.Card.ChatCount != scrumRealtimeChannelPageSize+10 {
			t.Fatalf("verbose card state leaked into realtime: %+v", message.Card)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for PostgreSQL-authoritative realtime message")
	}
}
