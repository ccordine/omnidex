package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestCanonicalRoleplayTurnAppliesSimulationNarratesCanonAndAdvancesAtomically(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "120")); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "simulation-story", Scope: model.ChannelScopeUser, Name: "Simulation story",
		WorkspaceRoot: "/srv/workspaces/simulation-story", Mode: model.ChannelModeRoleplay,
	}, "Crossing", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world found=%t err=%v", found, err)
	}
	ivo, err := store.CreateCharacter(ctx, world.ID, "Ivo")
	if err != nil {
		t.Fatal(err)
	}
	for _, persona := range []struct {
		id, summary, voice string
	}{
		{string(channel.RoleplayViewpointCharacterID), "A careful navigator.", "Measured."},
		{ivo.ID, "A patient lookout.", "Dry and direct."},
	} {
		if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
			CharacterID: persona.id, ExpectedRevision: 0,
			Sheet: roleplay.PersonaSheet{Summary: persona.summary, Voice: persona.voice, Traits: []string{}, Goals: []string{}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, meter := range []roleplay.MeterDefinition{
		{WorldID: world.ID, Key: "strain", Name: "Strain", Minimum: 0, Maximum: 100, InitialValue: 80},
		{WorldID: world.ID, Key: "morale", Name: "Morale", Minimum: 0, Maximum: 100, InitialValue: 40},
	} {
		if err := store.RegisterMeter(ctx, meter); err != nil {
			t.Fatal(err)
		}
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Observation deck",
		Description:    "A quiet room above the storm-lit city.",
		ParticipantIDs: []string{string(channel.RoleplayViewpointCharacterID), ivo.ID},
	}); err != nil {
		t.Fatal(err)
	}
	itemID, err := roleplay.NewItemTemplateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterItemTemplate(ctx, roleplay.ItemTemplateDefinition{
		ID: itemID, WorldID: world.ID, Name: "Field kit", Description: "A compact restorative kit.",
		UsePolicy: roleplay.ItemUseFinite, InitialUses: 1, Priority: 10,
		Trigger: &roleplay.ItemTrigger{MeterKey: "strain", Direction: roleplay.ThresholdAtOrAbove, Threshold: 70},
		Effects: []roleplay.MeterDelta{{MeterKey: "strain", Delta: -30}, {MeterKey: "morale", Delta: 5}},
	}); err != nil {
		t.Fatal(err)
	}

	message, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, `/give "Field kit"`)
	if err != nil {
		t.Fatal(err)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.RoleplayViewpointCharacterID != channel.RoleplayViewpointCharacterID ||
		metadata.RoleplayInputKind != roleplay.SimulationTurnAction || metadata.RoleplaySceneRevision != 2 ||
		len(metadata.RoleplayParticipantCharacterIDs) != 2 || metadata.RoleplaySimulationPreparationID == "" {
		t.Fatalf("prepared metadata=%+v", metadata)
	}
	var stagedRevision, stagedStrain, stagedMorale, stagedInventory, stagedTransitions int
	if err := pool.QueryRow(ctx, `
		SELECT scene.revision,
		       (SELECT value FROM roleplay_character_meters WHERE character_id=$2 AND meter_key='strain'),
		       (SELECT value FROM roleplay_character_meters WHERE character_id=$2 AND meter_key='morale'),
		       (SELECT COUNT(*) FROM roleplay_inventory_items WHERE character_id=$2),
		       (SELECT COUNT(*) FROM roleplay_simulation_transitions WHERE world_id=$1)
		FROM roleplay_current_scenes AS scene WHERE scene.world_id=$1
	`, world.ID, channel.RoleplayViewpointCharacterID).Scan(
		&stagedRevision, &stagedStrain, &stagedMorale, &stagedInventory, &stagedTransitions,
	); err != nil {
		t.Fatal(err)
	}
	if stagedRevision != 1 || stagedStrain != 80 || stagedMorale != 40 ||
		stagedInventory != 0 || stagedTransitions != 0 {
		t.Fatalf("enqueue published staged state revision=%d strain=%d morale=%d inventory=%d transitions=%d",
			stagedRevision, stagedStrain, stagedMorale, stagedInventory, stagedTransitions)
	}
	prepared, projected, err := repository.ProjectRoleplaySimulationContext(
		ctx, metadata.RoleplaySimulationPreparationID, job.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.UserMessageID != message.ID || len(projected.RecentEvents) != 2 ||
		!strings.Contains(strings.Join(projected.RecentEvents, "\n"), "Field kit") {
		t.Fatalf("preparation=%+v projection=%+v", prepared, projected)
	}
	if len(projected.Inventory) != 0 {
		t.Fatalf("exhausted finite item remained in narrative inventory: %+v", projected.Inventory)
	}
	meters := make(map[string]int, len(projected.Meters))
	for _, meter := range projected.Meters {
		meters[meter.Name] = meter.Value
	}
	if meters["Strain"] != 50 || meters["Morale"] != 45 {
		t.Fatalf("post-sieve meters=%v", meters)
	}

	claim, err := repository.ClaimNextStep(ctx, "simulation-proof-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v job=%d", claim, job.ID)
	}
	operationID := testLifecycleOperationID(t, "simulation-turn-complete", claim.Step.ID)
	fact := "Mara used the field kit on the observation deck."
	command := CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
		Output:     "Mara opens the compact kit and steadies herself as the storm rolls east.",
		ContextKey: "objective_result", ContextValue: "simulation-objective-proof",
		RoleplayFacts:                 []string{fact},
		RoleplayKnowledgeCharacterIDs: []model.RoleplayCharacterID{channel.RoleplayViewpointCharacterID},
	}
	completion := CompleteStepEvidenceCommand{CompleteStepCommand: command, Evidence: nil}
	if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
		t.Fatalf("exact completion replay: %v", err)
	}
	var activeCharacterID string
	var sceneRevision int64
	if err := pool.QueryRow(ctx, `
		SELECT current_character_id,revision FROM roleplay_current_scenes WHERE world_id=$1
	`, world.ID).Scan(&activeCharacterID, &sceneRevision); err != nil {
		t.Fatal(err)
	}
	if activeCharacterID != ivo.ID || sceneRevision != 3 {
		t.Fatalf("advanced active=%q revision=%d", activeCharacterID, sceneRevision)
	}
	var assistantMessages, transitions, preparations, advances int
	if err := pool.QueryRow(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant'),
		 (SELECT COUNT(*) FROM roleplay_simulation_transitions WHERE world_id=$2),
		 (SELECT COUNT(*) FROM roleplay_simulation_turn_preparations WHERE world_id=$2),
		 (SELECT COUNT(*) FROM roleplay_simulation_turn_advances WHERE world_id=$2)
	`, channel.ID, world.ID).Scan(&assistantMessages, &transitions, &preparations, &advances); err != nil {
		t.Fatal(err)
	}
	if assistantMessages != 1 || transitions != 1 || preparations != 1 || advances != 1 {
		t.Fatalf("assistant=%d transitions=%d preparations=%d advances=%d", assistantMessages, transitions, preparations, advances)
	}
	var eventID string
	if err := pool.QueryRow(ctx, `
		SELECT id FROM roleplay_canon_events WHERE world_id=$1 AND content=$2
	`, world.ID, fact).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	var memoryCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM roleplay_character_memories WHERE source_event_id=$1
	`, eventID).Scan(&memoryCount); err != nil {
		t.Fatal(err)
	}
	if memoryCount != 1 {
		t.Fatalf("exact replay persisted %d memories, want 1", memoryCount)
	}
	reopenedStore, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	maraProjection, _, err := reopenedStore.ProjectSimulationNarrative(
		ctx, world.ID, string(channel.RoleplayViewpointCharacterID),
	)
	if err != nil {
		t.Fatal(err)
	}
	ivoProjection, _, err := reopenedStore.ProjectSimulationNarrative(ctx, world.ID, ivo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(maraProjection.Memories) != 1 || maraProjection.Memories[0] != fact ||
		len(ivoProjection.Memories) != 0 {
		t.Fatalf("memory isolation mara=%v ivo=%v", maraProjection.Memories, ivoProjection.Memories)
	}

	_, failedJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, `/give "Field kit"`)
	if err != nil {
		t.Fatal(err)
	}
	failedClaim, err := repository.ClaimNextStep(ctx, "simulation-proof-worker-failure")
	if err != nil {
		t.Fatal(err)
	}
	if failedClaim == nil || failedClaim.Job.ID != failedJob.ID {
		t.Fatalf("failed-turn claim=%+v job=%d", failedClaim, failedJob.ID)
	}
	if err := repository.FailStep(ctx, FailStepCommand{
		OperationID: testLifecycleOperationID(t, "simulation-narrative-failure", failedClaim.Step.ID),
		Authority:   failedClaim.Authority, StepID: failedClaim.Step.ID,
		Error: "injected narrative failure",
	}); err != nil {
		t.Fatal(err)
	}
	var failedRevision, failedStrain, failedMorale, failedInventory int
	var failedTransitions, failedAssistants, failedCanon, failedAdvances int
	if err := pool.QueryRow(ctx, `
		SELECT scene.revision,
		       (SELECT value FROM roleplay_character_meters WHERE character_id=$2 AND meter_key='strain'),
		       (SELECT value FROM roleplay_character_meters WHERE character_id=$2 AND meter_key='morale'),
		       (SELECT COUNT(*) FROM roleplay_inventory_items WHERE character_id=$2),
		       (SELECT COUNT(*) FROM roleplay_simulation_transitions WHERE world_id=$1),
		       (SELECT COUNT(*) FROM ai_channel_messages WHERE channel_id=$3 AND role='assistant'),
		       (SELECT COUNT(*) FROM roleplay_canon_events WHERE world_id=$1),
		       (SELECT COUNT(*) FROM roleplay_simulation_turn_advances WHERE world_id=$1)
		FROM roleplay_current_scenes AS scene WHERE scene.world_id=$1
	`, world.ID, ivo.ID, channel.ID).Scan(
		&failedRevision, &failedStrain, &failedMorale, &failedInventory,
		&failedTransitions, &failedAssistants, &failedCanon, &failedAdvances,
	); err != nil {
		t.Fatal(err)
	}
	if failedRevision != 3 || failedStrain != 80 || failedMorale != 40 || failedInventory != 0 ||
		failedTransitions != 1 || failedAssistants != 1 || failedCanon != 1 || failedAdvances != 1 {
		t.Fatalf("failed turn published state revision=%d strain=%d morale=%d inventory=%d transitions=%d assistants=%d canon=%d advances=%d",
			failedRevision, failedStrain, failedMorale, failedInventory, failedTransitions,
			failedAssistants, failedCanon, failedAdvances)
	}

	_, nextJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Ivo watches the horizon.")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(nextJob.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.RoleplayViewpointCharacterID != model.RoleplayCharacterID(ivo.ID) ||
		metadata.RoleplayInputKind != roleplay.SimulationTurnProse {
		t.Fatalf("next prepared turn=%+v", metadata)
	}
	nextClaim, err := repository.ClaimNextStep(ctx, "simulation-proof-worker-next")
	if err != nil {
		t.Fatal(err)
	}
	if nextClaim == nil || nextClaim.Job.ID != nextJob.ID {
		t.Fatalf("next claim=%+v job=%d", nextClaim, nextJob.ID)
	}
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, "simulation-prose-complete", nextClaim.Step.ID),
			Authority:   nextClaim.Authority, StepID: nextClaim.Step.ID,
			Output:     "Ivo declares that his morale is now ninety-nine.",
			ContextKey: "objective_result", ContextValue: "simulation-prose-proof",
		},
		Evidence: nil,
	}); err != nil {
		t.Fatal(err)
	}
	var ivoMorale int
	if err := pool.QueryRow(ctx, `
		SELECT value FROM roleplay_character_meters
		WHERE character_id=$1 AND meter_key='morale'
	`, ivo.ID).Scan(&ivoMorale); err != nil {
		t.Fatal(err)
	}
	if ivoMorale != 40 {
		t.Fatalf("assistant prose mutated authoritative meter to %d", ivoMorale)
	}
}
