package queue

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresScrumChannelReceiptSurvivesCardDeletion(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "delete")
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-delete", project.ID),
		ProjectID:   project.ID,
		CardID:      card.ID,
		Message:     "Start before deleting this card.",
	}
	command := ScrumChannelOperationCommand{
		Request:               request,
		ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect: ScrumChannelEffect{
			Kind:        ScrumChannelStartJob,
			Instruction: request.Message,
			Pipeline:    model.PipelineAssistant,
			Metadata:    json.RawMessage(fmt.Sprintf(`{"project_id":%d}`, project.ID)),
		},
		ResultAction: "started",
		ResultAgent:  "omnidex",
	}
	result, err := repository.ExecuteScrumChannelOperation(
		ctx,
		command,
		func(current DBScrumCard, job model.Job) (ScrumChannelCardUpdate, error) {
			return scrumChannelTestUpdate(t, current, request, job), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteScrumCard(ctx, project.ID, card.ID); err != nil {
		t.Fatalf("delete card with immutable channel receipt: %v", err)
	}
	replay, found, err := repository.LoadScrumChannelOperation(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !found || replay.Applied || replay.Job.ID != result.Job.ID ||
		replay.Card.ID != result.Card.ID || string(replay.Card.Chat) != string(result.Card.Chat) {
		t.Fatalf("receipt replay after card deletion=%+v found=%t", replay, found)
	}
}
