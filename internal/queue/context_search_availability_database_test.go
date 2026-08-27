package queue

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplaySearchAvailabilityDistinguishesExactRecentBoundFromOneOlderRound(
	t *testing.T,
) {
	fixtures := []struct {
		name           string
		historyRounds  int
		wantAdditional bool
	}{
		{name: "exact bound has no hidden round", historyRounds: 4, wantAdditional: false},
		{name: "one older round is searchable", historyRounds: 5, wantAdditional: true},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, repository := channelTurnTestRepository(t)
			channelID := model.ChannelID(fmt.Sprintf(
				"roleplay-search-availability-%d", fixture.historyRounds,
			))
			channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
				ID: channelID, Scope: model.ChannelScopeUser,
				Name:          "Roleplay search availability",
				WorkspaceRoot: "/srv/workspaces/" + string(channelID),
				Mode:          model.ChannelModeRoleplay,
			}, "Availability", "Mara")
			if err != nil {
				t.Fatal(err)
			}
			store, err := roleplay.NewStore(repository.pool)
			if err != nil {
				t.Fatal(err)
			}
			world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
			if err != nil || !found {
				t.Fatalf("world found=%t err=%v", found, err)
			}
			viewpointID := string(channel.RoleplayViewpointCharacterID)
			if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
				CharacterID: viewpointID, ExpectedRevision: 0,
				Sheet: roleplay.PersonaSheet{
					Summary: "A watch officer.", Voice: "Direct.",
					Traits: []string{}, Goals: []string{},
				},
			}); err != nil {
				t.Fatal(err)
			}
			sceneID, err := roleplay.NewSceneIdentity()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
				ID: sceneID, WorldID: world.ID, Title: "Archive",
				Description:    "A quiet room with one observer.",
				ParticipantIDs: []string{viewpointID},
			}); err != nil {
				t.Fatal(err)
			}
			for index := 0; index < fixture.historyRounds; index++ {
				_, job, err := enqueueNarratorRoleplayTurn(
					ctx, repository, channel.ID,
					fmt.Sprintf("History round %d enters the archive.", index),
				)
				if err != nil {
					t.Fatal(err)
				}
				completeRoleplayConversationRound(t, repository, job, map[string]string{
					viewpointID: fmt.Sprintf("Mara records history round %d.", index),
				}, "", "")
			}
			_, currentJob, err := enqueueNarratorRoleplayTurn(
				ctx, repository, channel.ID, "Which earlier archive round matters?",
			)
			if err != nil {
				t.Fatal(err)
			}
			recent, err := repository.RoleplayConversationCandidateAuthorities(
				ctx, currentJob, channel.RoleplayViewpointCharacterID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(roleplayConversationSearchSourceIDs(recent)) != 4 {
				t.Fatalf("recent roleplay suffix=%#v", recent)
			}
			projection, narrativeAuthority, err := store.ProjectSimulationNarrative(
				ctx, world.ID, viewpointID,
			)
			if err != nil {
				t.Fatal(err)
			}
			var metadata channelTurnMetadata
			if err := json.Unmarshal(currentJob.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			preparation, _, err := repository.ProjectRoleplaySimulationContext(
				ctx, metadata.RoleplaySimulationPreparationID, currentJob.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			additional, err := repository.HasAdditionalRoleplaySearchAuthority(
				ctx, world.ID, channel.RoleplayViewpointCharacterID, sceneID,
				preparation.CreatedAt, RoleplayContextSearchRepresentation{
					CanonEventIDs:           narrativeAuthority.CanonEventIDs,
					MemoryIDs:               narrativeAuthority.MemoryIDs,
					ConversationSourceIDs:   roleplayConversationSearchSourceIDs(recent),
					SimulationEventContents: projection.RecentEvents,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if additional != fixture.wantAdditional {
				t.Fatalf(
					"history rounds=%d additional=%t want %t",
					fixture.historyRounds, additional, fixture.wantAdditional,
				)
			}
		})
	}
}
