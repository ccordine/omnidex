package roleplay

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleplaySimulationPersistsDeterministicTransitionsAndTurnAuthority(t *testing.T) {
	pool, reopen := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, ari := bootstrapRoleplayChannel(t, pool, "simulation-channel", "Crossroads", "Ari")
	bex, err := store.CreateCharacter(ctx, world.ID, "Bex")
	if err != nil {
		t.Fatal(err)
	}
	writeTestPersona(t, store, ari.ID, "A patient traveler.")
	writeTestPersona(t, store, bex.ID, "A watchful traveler.")
	for _, meter := range []MeterDefinition{
		{WorldID: world.ID, Key: "energy", Name: "Energy", Minimum: 0, Maximum: 10, InitialValue: 3},
		{WorldID: world.ID, Key: "affinity", Name: "Affinity", Minimum: 0, Maximum: 10, InitialValue: 8},
	} {
		if err := store.RegisterMeter(ctx, meter); err != nil {
			t.Fatal(err)
		}
	}
	commandID, _ := NewInteractionCommandIdentity()
	if err := store.RegisterInteractionCommand(ctx, InteractionCommandDefinition{
		ID: commandID, WorldID: world.ID, Key: "encourage", Name: "Encourage",
		Description: "A kind word restores confidence.", ArgumentMode: CommandArgumentRequired,
		Effects: []MeterDelta{{MeterKey: "affinity", Delta: 5}},
	}); err != nil {
		t.Fatal(err)
	}
	registerTestItems(t, store, world.ID)
	assertDatabaseRejectsUnaddressableItemNames(t, pool, world.ID)
	roundTripItemID, err := NewItemTemplateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	const roundTripItemName = "Traveler's Ω kit [Mk II]"
	if err := store.RegisterItemTemplate(ctx, ItemTemplateDefinition{
		ID: roundTripItemID, WorldID: world.ID, Name: roundTripItemName,
		Description: "A task-neutral item-name round-trip fixture.",
		UsePolicy:   ItemUseInfinite, Effects: []MeterDelta{{MeterKey: "affinity", Delta: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	sceneID, _ := NewSceneIdentity()
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Night camp",
		Description: "Two travelers share a small fire.", ParticipantIDs: []string{ari.ID, bex.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	keep := applyTestAction(t, store, world.ID, scene.ID, scene.Revision, `/give "Keepsake"`)
	if len(keep.Effects) != 1 || keep.Effects[0].Kind != "inventory_added" {
		t.Fatalf("inert give effects=%+v", keep.Effects)
	}
	inventory, err := store.ListInventoryPage(ctx, world.ID, ari.ID, 10, 0)
	if err != nil || len(inventory.Items) != 1 || inventory.Items[0].RemainingUses != 2 {
		t.Fatalf("inert inventory=%+v error=%v", inventory, err)
	}
	giveExact, err := CanonicalItemAction(SimulationActionGive, roundTripItemName)
	if err != nil {
		t.Fatal(err)
	}
	roundTripGiven := applyTestAction(t, store, world.ID, scene.ID, keep.AfterRevision, giveExact)
	takeExact, err := CanonicalItemAction(SimulationActionTake, roundTripItemName)
	if err != nil {
		t.Fatal(err)
	}
	roundTripTaken := applyTestAction(t, store, world.ID, scene.ID, roundTripGiven.AfterRevision, takeExact)
	ration := applyTestAction(t, store, world.ID, scene.ID, roundTripTaken.AfterRevision, `/give "Ration"`)
	if len(ration.NarrativeEvents) != 2 || ration.Effects[len(ration.Effects)-1].Kind != "item_auto_used_and_exhausted" {
		t.Fatalf("ration transition=%+v", ration)
	}
	meters, err := store.ListViewpointMetersPage(ctx, world.ID, ari.ID, 10, 0)
	if err != nil || meterValue(meters, "energy") != 10 {
		t.Fatalf("clamped meters=%+v error=%v", meters, err)
	}
	encourage := applyTestAction(t, store, world.ID, scene.ID, ration.AfterRevision, `/encourage "Hold steady."`)
	if meterAfter(encourage, "affinity") != 10 || encourage.NarrativeEvents[0] != "A kind word restores confidence.\nDetail: Hold steady." {
		t.Fatalf("encourage transition=%+v", encourage)
	}
	taken := applyTestAction(t, store, world.ID, scene.ID, encourage.AfterRevision, `/take "Keepsake"`)
	tinnitus := applyTestAction(t, store, world.ID, scene.ID, taken.AfterRevision, `/give "Tonic"`)
	if len(tinnitus.Effects) != 1 {
		t.Fatalf("ineligible tonic effects=%+v", tinnitus.Effects)
	}
	meters, err = store.ListViewpointMetersPage(ctx, world.ID, ari.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	energy := meterProjection(meters, "energy")
	if _, err := store.SetCharacterMeter(ctx, MeterValueUpdate{
		WorldID: world.ID, CharacterID: ari.ID, MeterKey: "energy",
		ExpectedRevision: energy.Revision, Value: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyTestActionResult(
		t, store, world.ID, scene.ID, tinnitus.AfterRevision, "/unregistered",
	); !errors.Is(err, ErrSimulationUnknown) {
		t.Fatalf("unknown action error=%v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channel_messages (id,channel_id,role,content) VALUES
		(91,$1,'assistant','The fire gutters.'),(92,$1,'user','The night grows colder.')
	`, world.ChannelID); err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendCanonEvent(ctx, world.ID, 91, "Ari saw the campfire gutter.")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.GrantKnowledge(ctx, ari.ID, event.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterMemory(ctx, ari.ID, event.ID, "The failing fire made Ari uneasy."); err != nil {
		t.Fatal(err)
	}
	authority := prepareAndBindTestTurn(t, pool, world.ChannelID, 92, 201, "The night grows colder.")
	if authority.PendingTransition == nil || authority.BaseSceneRevision != tinnitus.AfterRevision ||
		authority.SceneRevision != tinnitus.AfterRevision+1 {
		t.Fatalf("prepared authority=%+v", authority)
	}
	loaded, err := store.LoadSimulationTurnForJob(ctx, authority.PreparationID, 201)
	if err != nil || loaded.NarrativeFingerprint != authority.NarrativeFingerprint {
		t.Fatalf("loaded authority=%+v error=%v", loaded, err)
	}
	content := authority.NarrativeProjection
	contentAuthority := authority.NarrativeAuthority
	if content.Validate() != nil || contentAuthority.Fingerprint != authority.NarrativeFingerprint {
		t.Fatalf("narrative content=%+v authority=%+v", content, contentAuthority)
	}
	if len(content.VisibleFacts) != 1 || len(content.Memories) != 1 || content.RecentEvents[len(content.RecentEvents)-1] != "Used Tonic. A restorative travel tonic." {
		t.Fatalf("narrative content=%+v", content)
	}
	advanceID := mustTransitionID(t)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := MaterializeSimulationTurnTx(ctx, tx, SimulationTurnMaterializationRequest{
		PreparationID: authority.PreparationID, ChannelID: world.ChannelID,
		UserMessageID: 92, JobID: 201,
	}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	advance, err := AdvanceTurnTx(ctx, tx, SimulationTurnAdvanceRequest{
		OperationID: advanceID, PreparationID: authority.PreparationID,
		ChannelID: world.ChannelID, UserMessageID: 92, JobID: 201, ExpectedRevision: authority.SceneRevision,
	})
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var materializedPayload []byte
	if err := pool.QueryRow(ctx, `
		SELECT result FROM roleplay_simulation_transitions WHERE operation_id=$1
	`, authority.PendingTransition.OperationID).Scan(&materializedPayload); err != nil {
		t.Fatal(err)
	}
	materializedTransition, err := decodeSimulationTransitionResult(materializedPayload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*authority.PendingTransition, materializedTransition) {
		t.Fatalf("materialized transition differs from exact preparation\nprepared=%+v\nactual=%+v",
			*authority.PendingTransition, materializedTransition)
	}
	if advance.ActiveCharacterID != bex.ID || advance.PreviousCharacterID != ari.ID {
		t.Fatalf("advance=%+v", advance)
	}
	replayTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replayedAdvance, err := AdvanceTurnTx(ctx, replayTx, SimulationTurnAdvanceRequest{
		OperationID: advanceID, PreparationID: authority.PreparationID,
		ChannelID: world.ChannelID, UserMessageID: 92, JobID: 201, ExpectedRevision: authority.SceneRevision,
	})
	if rollbackErr := replayTx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err != nil || replayedAdvance.AfterRevision != advance.AfterRevision ||
		replayedAdvance.NarrativeFingerprint != advance.NarrativeFingerprint {
		t.Fatalf("advance replay=%+v error=%v", replayedAdvance, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channel_messages (id,channel_id,role,content)
		VALUES (93,$1,'user','Bex listens in silence.')
	`, world.ChannelID); err != nil {
		t.Fatal(err)
	}
	quiet := prepareAndBindTestTurn(t, pool, world.ChannelID, 93, 202, "Bex listens in silence.")
	if quiet.PendingTransition != nil || quiet.BaseSceneRevision != advance.AfterRevision ||
		quiet.SceneRevision != advance.AfterRevision {
		t.Fatalf("zero-delta prose authority=%+v", quiet)
	}
	var zeroDeltaTransitions int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM roleplay_simulation_transitions WHERE operation_id=$1
	`, quiet.PreparationID).Scan(&zeroDeltaTransitions); err != nil {
		t.Fatal(err)
	}
	if zeroDeltaTransitions != 0 {
		t.Fatalf("zero-delta prose persisted transitions=%d", zeroDeltaTransitions)
	}

	pool.Close()
	reopened := reopen(t)
	t.Cleanup(reopened.Close)
	reopenedStore, err := NewStore(reopened)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reopenedStore.ProjectCurrentScene(ctx, world.ID)
	if err != nil || persisted.ActiveCharacterID != bex.ID || persisted.Revision != advance.AfterRevision {
		t.Fatalf("persisted scene=%+v error=%v", persisted, err)
	}
}

func assertDatabaseRejectsUnaddressableItemNames(
	t *testing.T,
	pool *pgxpool.Pool,
	worldID string,
) {
	t.Helper()
	for _, name := range []string{`quoted "item"`, `back\\slash`, "line\nbreak", "carriage\rreturn"} {
		id, err := NewItemTemplateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), `
			INSERT INTO roleplay_item_templates (
				id,world_id,name,description,use_policy,priority
			) VALUES ($1,$2,$3,'A rejected fixture.','infinite',0)
		`, id, worldID, name); err == nil {
			t.Fatalf("database accepted unaddressable item name %q", name)
		}
	}
}

func installSimulationTestSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `ALTER TABLE jobs ADD COLUMN instruction TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "118_roleplay_simulation_authority.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), string(body)); err != nil {
		t.Fatalf("install simulation migration: %v", err)
	}
}
