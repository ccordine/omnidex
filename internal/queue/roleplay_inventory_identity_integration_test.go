package queue

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestInventoryIdentityFollowsItsPreparedTurnWithoutContentHashes(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	for _, fixture := range []struct {
		name, meter string
		policy      roleplay.ItemUsePolicy
		uses        int
	}{
		{"Instrument", "heat", roleplay.ItemUseFinite, 2},
		{"Notebook", "confidence", roleplay.ItemUseInfinite, 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			pool, repository := freshEvidenceRepository(t, databaseURL)
			channel, world, store := roleplayEvidenceScene(t, repository)
			characterID := string(channel.RoleplayViewpointCharacterID)
			if err := store.RegisterMeter(ctx, roleplay.MeterDefinition{
				WorldID: world.ID, Key: fixture.meter, Name: fixture.meter, Minimum: 0, Maximum: 10, InitialValue: 3,
			}); err != nil {
				t.Fatal(err)
			}
			templateID, err := roleplay.NewItemTemplateIdentity()
			if err != nil {
				t.Fatal(err)
			}
			if err := store.RegisterItemTemplate(ctx, roleplay.ItemTemplateDefinition{
				ID: templateID, WorldID: world.ID, Name: fixture.name, Description: "A configured item.",
				UsePolicy: fixture.policy, InitialUses: fixture.uses, Effects: []roleplay.MeterDelta{{MeterKey: fixture.meter, Delta: 1}},
			}); err != nil {
				t.Fatal(err)
			}
			var previousInventoryID string
			for round, kind := range []roleplay.SimulationActionKind{roleplay.SimulationActionGive, roleplay.SimulationActionTake, roleplay.SimulationActionGive} {
				exact, err := roleplay.CanonicalItemAction(kind, fixture.name)
				if err != nil {
					t.Fatal(err)
				}
				_, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, exact, roleplay.UserTurnRequest{
					PersonaKind: roleplay.UserPersonaNarrator, ContributionKind: roleplay.UserContributionCommand,
				})
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "inventory-identity-test")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim = %#v / %v", claim, err)
				}
				var metadata channelTurnMetadata
				if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
					t.Fatal(err)
				}
				prepared, err := store.LoadFreshSimulationTurnForJob(ctx, metadata.RoleplaySimulationPreparationID, job.ID)
				if err != nil {
					t.Fatal(err)
				}
				wantCount := 0
				if kind == roleplay.SimulationActionGive {
					wantCount = 1
					wantID := "rpv_" + strings.TrimPrefix(prepared.PreparationID, "rpt_")
					if !reflect.DeepEqual(prepared.NarrativeAuthority.InventoryItemIDs, []string{wantID}) || wantID == previousInventoryID {
						t.Fatalf("preview inventory identity is not owned by this turn: %#v", prepared.NarrativeAuthority.InventoryItemIDs)
					}
					previousInventoryID = wantID
				}
				operationID, err := model.NewLifecycleOperationID()
				if err != nil {
					t.Fatal(err)
				}
				completion := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
					OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
					Output: "The action is complete.", ContextKey: "objective_result",
					RoleplayResponses: []RoleplayResponseCompletion{{
						Position: 0, CharacterID: channel.RoleplayViewpointCharacterID, Output: "The action is complete.",
						Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
					}},
				}}
				for replay := 0; replay < 2; replay++ {
					if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
						t.Fatal(err)
					}
				}
				var count int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM roleplay_inventory_items WHERE character_id=$1`, characterID).Scan(&count); err != nil || count != wantCount {
					t.Fatalf("inventory count=%d want=%d error=%v", count, wantCount, err)
				}
				if wantCount == 1 {
					var actualID string
					var remaining *int
					if err := pool.QueryRow(ctx, `SELECT id,remaining_uses FROM roleplay_inventory_items WHERE character_id=$1 AND template_id=$2`, characterID, templateID).Scan(&actualID, &remaining); err != nil {
						t.Fatal(err)
					}
					if actualID != previousInventoryID || fixture.policy == roleplay.ItemUseFinite && (remaining == nil || *remaining != fixture.uses) ||
						fixture.policy == roleplay.ItemUseInfinite && remaining != nil {
						t.Fatalf("published inventory differs from preview: id=%q uses=%v", actualID, remaining)
					}
				}
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM roleplay_simulation_turn_advances WHERE world_id=$1`, world.ID).Scan(&count); err != nil || count != round+1 {
					t.Fatalf("replay changed advancement count=%d error=%v", count, err)
				}
			}
		})
	}
}
