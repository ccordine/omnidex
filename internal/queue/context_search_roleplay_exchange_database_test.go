package queue

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayContextSearchReturnsOneCompleteRoundInRecentDedupShape(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "roleplay-search-complete-round", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeRoleplay, Name: "Roleplay complete-round search",
		WorkspaceRoot: "/srv/workspaces/roleplay-search-complete-round",
	}, "Complete Round", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("resolve roleplay world: found=%t err=%v", found, err)
	}
	ivo, err := store.CreateCharacter(ctx, world.ID, "Ivo")
	if err != nil {
		t.Fatal(err)
	}
	maraID := string(channel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, world.ID, maraID, ivo.ID)
	userMessage, completedJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID,
		"The quartz chronometer begins its sidereal cadence.",
	)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{
		maraID: "Mara checks the quartz chronometer against the north dial.",
		ivo.ID: "Ivo records the sidereal cadence in his field ledger.",
	}
	completeRoleplayConversationRound(t, repository, completedJob, outputs, "", "")
	_, currentJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "What does the complete prior round establish?",
	)
	if err != nil {
		t.Fatal(err)
	}
	currentPreparation, _, err := repository.ProjectRoleplaySimulationContext(
		ctx, roleplayPreparationID(t, currentJob), currentJob.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantContent := "Narrator [direction] contribution:\n" + userMessage.Content +
		"\nMara response:\n" + outputs[maraID] +
		"\nIvo response:\n" + outputs[ivo.ID]
	for _, terms := range [][]string{{"quartz chronometer"}, {"field ledger"}} {
		records, err := repository.SearchRoleplayContextRecords(
			ctx, world.ID, channel.RoleplayViewpointCharacterID,
			currentPreparation.SceneID, currentPreparation.CreatedAt, terms, 8,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 || records[0].Namespace != "conversation_exchange" ||
			records[0].Content != wantContent ||
			!strings.HasPrefix(
				records[0].SourceID,
				"channel-message-"+strconv.FormatInt(userMessage.ID, 10)+"-through-",
			) {
			t.Fatalf("searched complete round for %q=%#v, want content %q", terms, records, wantContent)
		}
	}
}

func roleplayPreparationID(t *testing.T, job model.Job) string {
	t.Helper()
	binding, exists, err := channelBindingForJob(job)
	if err != nil || !exists || binding.RoleplaySimulationPreparationID == "" {
		t.Fatalf("resolve roleplay preparation binding: exists=%t err=%v", exists, err)
	}
	return binding.RoleplaySimulationPreparationID
}
