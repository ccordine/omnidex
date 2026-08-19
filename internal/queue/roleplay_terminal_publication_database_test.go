package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPreparedRoleplaySimulationCannotPublishBeforeTerminalCompletion(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "120")); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "terminal-simulation-guard", Scope: model.ChannelScopeUser,
		Name: "Terminal simulation guard", WorkspaceRoot: "/srv/workspaces/terminal-simulation-guard",
		Mode: model.ChannelModeRoleplay,
	}, "Guarded world", "Ari")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("resolve guarded world: found=%t err=%v", found, err)
	}
	characterID := string(channel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, world.ID, characterID)
	if err := store.RegisterMeter(ctx, roleplay.MeterDefinition{
		WorldID: world.ID, Key: "focus", Name: "Focus", Minimum: 0, Maximum: 10, InitialValue: 5,
	}); err != nil {
		t.Fatal(err)
	}
	commandID, err := roleplay.NewInteractionCommandIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterInteractionCommand(ctx, roleplay.InteractionCommandDefinition{
		ID: commandID, WorldID: world.ID, Key: "settle", Name: "Settle",
		Description: "The character regains composure.", ArgumentMode: roleplay.CommandArgumentNone,
		Effects: []roleplay.MeterDelta{{MeterKey: "focus", Delta: -2}},
	}); err != nil {
		t.Fatal(err)
	}

	_, actionJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "/settle")
	if err != nil {
		t.Fatal(err)
	}
	actionBinding := decodeTerminalGuardBinding(t, actionJob)
	materializationTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer materializationTx.Rollback(ctx)
	if err := roleplay.MaterializeSimulationTurnTx(ctx, materializationTx, roleplay.SimulationTurnMaterializationRequest{
		PreparationID: actionBinding.RoleplaySimulationPreparationID,
		ChannelID:     string(channel.ID), UserMessageID: actionBinding.ChannelUserMessageID,
		JobID: actionJob.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := materializationTx.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "prepared simulation transition requires one exact terminal roleplay completion") {
		t.Fatalf("standalone materialization commit error=%v", err)
	}
	assertTerminalGuardState(t, pool, world.ID, characterID, 1, 5, 0, 0)
	actionClaim, err := repository.ClaimNextStep(ctx, "terminal-simulation-action-failure")
	if err != nil {
		t.Fatal(err)
	}
	if actionClaim == nil || actionClaim.Job.ID != actionJob.ID {
		t.Fatalf("action claim=%+v job=%d", actionClaim, actionJob.ID)
	}
	if err := repository.FailStep(ctx, FailStepCommand{
		OperationID: testLifecycleOperationID(t, "terminal-simulation-action-failure", actionClaim.Step.ID),
		Authority:   actionClaim.Authority, StepID: actionClaim.Step.ID,
		Error: "injected terminal-publication proof failure",
	}); err != nil {
		t.Fatal(err)
	}
	assertTerminalGuardState(t, pool, world.ID, characterID, 1, 5, 0, 0)

	_, quietJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Ari waits in silence.")
	if err != nil {
		t.Fatal(err)
	}
	quietBinding := decodeTerminalGuardBinding(t, quietJob)
	advanceID, err := roleplay.NewSimulationTransitionIdentity()
	if err != nil {
		t.Fatal(err)
	}
	advanceTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer advanceTx.Rollback(ctx)
	if _, err := roleplay.AdvanceTurnTx(ctx, advanceTx, roleplay.SimulationTurnAdvanceRequest{
		OperationID: advanceID, PreparationID: quietBinding.RoleplaySimulationPreparationID,
		ChannelID: string(channel.ID), UserMessageID: quietBinding.ChannelUserMessageID,
		JobID: quietJob.ID, ExpectedRevision: quietBinding.RoleplaySceneRevision,
	}); err != nil {
		t.Fatal(err)
	}
	if err := advanceTx.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "simulation turn advance requires one exact terminal roleplay completion") {
		t.Fatalf("standalone turn-advance commit error=%v", err)
	}
	assertTerminalGuardState(t, pool, world.ID, characterID, 1, 5, 0, 0)
}

func decodeTerminalGuardBinding(t *testing.T, job model.Job) channelTurnMetadata {
	t.Helper()
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	return binding
}

func assertTerminalGuardState(
	t *testing.T,
	pool *pgxpool.Pool,
	worldID, characterID string,
	wantRevision int64,
	wantMeter, wantTransitions, wantAdvances int,
) {
	t.Helper()
	var revision int64
	var meter, transitions, advances int
	if err := pool.QueryRow(t.Context(), `
		SELECT scene.revision,
		       (SELECT value FROM roleplay_character_meters
		        WHERE character_id=$2 AND meter_key='focus'),
		       (SELECT COUNT(*) FROM roleplay_simulation_transitions WHERE world_id=$1),
		       (SELECT COUNT(*) FROM roleplay_simulation_turn_advances WHERE world_id=$1)
		FROM roleplay_current_scenes AS scene WHERE scene.world_id=$1
	`, worldID, characterID).Scan(&revision, &meter, &transitions, &advances); err != nil {
		t.Fatal(err)
	}
	if revision != wantRevision || meter != wantMeter || transitions != wantTransitions || advances != wantAdvances {
		t.Fatalf("guarded state revision=%d meter=%d transitions=%d advances=%d",
			revision, meter, transitions, advances)
	}
}
