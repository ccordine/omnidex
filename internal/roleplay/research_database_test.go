package roleplay

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleplayResearchCapabilityIsPerCharacterRevocableAndReserved(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	installResearchTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, ari := bootstrapRoleplayChannel(t, pool, "research-channel", "Archive", "Ari")
	bex, err := store.CreateCharacter(ctx, world.ID, "Bex")
	if err != nil {
		t.Fatal(err)
	}
	writeTestPersona(t, store, ari.ID, "A careful archivist.")
	writeTestPersona(t, store, bex.ID, "A skeptical baker.")
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Reading room",
		Description:    "Two researchers sit beside a window.",
		ParticipantIDs: []string{ari.ID, bex.ID},
	}); err != nil {
		t.Fatal(err)
	}
	for _, meter := range []MeterDefinition{
		{WorldID: world.ID, Key: "energy", Name: "Energy", Minimum: 0, Maximum: 10, InitialValue: 5},
		{WorldID: world.ID, Key: "affinity", Name: "Affinity", Minimum: 0, Maximum: 10, InitialValue: 5},
	} {
		if err := store.RegisterMeter(ctx, meter); err != nil {
			t.Fatal(err)
		}
	}
	registerTestItems(t, store, world.ID)
	given := applyTestAction(t, store, world.ID, sceneID, 1, `/give "Tonic"`)
	meters, err := store.ListViewpointMetersPage(ctx, world.ID, ari.ID, 10, 0)
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
	if given.OperationID == "" {
		t.Fatal("fixture give did not create its deterministic simulation transition")
	}
	const exact = `/research "How do ocean currents redistribute heat?"`
	insertNarratorRoleplayUserMessage(
		t, pool, 301, world.ChannelID, exact, UserContributionCommand,
	)

	if err := authorizeResearchDatabaseFixture(ctx, pool, world.ChannelID, 301, exact); !errors.Is(err, ErrResearchCapabilityDenied) {
		t.Fatalf("capability absence error=%v", err)
	}
	bexProjection, err := store.ConfigureCharacterCapability(
		ctx, world.ID, bex.ID, CapabilityWebResearch, true,
	)
	if err != nil || !bexProjection.WebResearch {
		t.Fatalf("enable Bex projection=%+v err=%v", bexProjection, err)
	}
	if err := authorizeResearchDatabaseFixture(ctx, pool, world.ChannelID, 301, exact); !errors.Is(err, ErrResearchCapabilityDenied) {
		t.Fatalf("other-character capability leaked to active Ari: %v", err)
	}
	ariProjection, err := store.ConfigureCharacterCapability(
		ctx, world.ID, ari.ID, CapabilityWebResearch, true,
	)
	if err != nil || !ariProjection.WebResearch || ariProjection.WebResearchGrant == "" {
		t.Fatalf("enable Ari projection=%+v err=%v", ariProjection, err)
	}
	authority, err := persistResearchDatabaseFixture(ctx, pool, world.ChannelID, 301, exact)
	if err != nil {
		t.Fatal(err)
	}
	if authority.CharacterID != ari.ID || authority.Authority != AuthorityRealWorld ||
		authority.CapabilityGrantID != ariProjection.WebResearchGrant {
		t.Fatalf("research authority escaped active character or REAL_WORLD namespace: %+v", authority)
	}
	var explicitAction bool
	var pendingTransitionID *string
	if err := pool.QueryRow(ctx, `
		SELECT explicit_action,pending_transition_id
		FROM roleplay_simulation_turn_preparations WHERE operation_id=$1
	`, authority.PreparationID).Scan(&explicitAction, &pendingTransitionID); err != nil {
		t.Fatal(err)
	}
	if explicitAction || pendingTransitionID == nil || *pendingTransitionID == "" {
		t.Fatalf("external research did not preserve deterministic auto-use: explicit=%t transition=%v",
			explicitAction, pendingTransitionID)
	}
	disabled, err := store.ConfigureCharacterCapability(
		ctx, world.ID, ari.ID, CapabilityWebResearch, false,
	)
	if err != nil || disabled.WebResearch {
		t.Fatalf("disable Ari projection=%+v err=%v", disabled, err)
	}
	if _, found, err := findResearchPreparationForTest(ctx, pool, authority.PreparationID); err != nil || found {
		t.Fatalf("revoked grant remained executable: found=%t err=%v", found, err)
	}
	historicalTx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	historical, found, historicalErr := FindResearchTurnBindingForJobTx(ctx, historicalTx, 401)
	historicalTx.Rollback(ctx)
	if historicalErr != nil || !found || historical.CapabilityGrantID != authority.CapabilityGrantID {
		t.Fatalf("revoked historical binding=%+v found=%t error=%v", historical, found, historicalErr)
	}
	reissued, err := store.ConfigureCharacterCapability(
		ctx, world.ID, ari.ID, CapabilityWebResearch, true,
	)
	if err != nil || reissued.WebResearchGrant == ariProjection.WebResearchGrant {
		t.Fatalf("reissued capability did not receive fresh grant: before=%+v after=%+v err=%v",
			ariProjection, reissued, err)
	}

	commandID, _ := NewInteractionCommandIdentity()
	if _, err := pool.Exec(ctx, `
		INSERT INTO roleplay_interaction_commands (
			id,world_id,command_key,name,description,argument_mode
		) VALUES ($1,$2,'research','Research','conflict','none')
	`, commandID, world.ID); err == nil {
		t.Fatal("database accepted fictional interaction command in reserved research namespace")
	}
}

func authorizeResearchDatabaseFixture(
	ctx context.Context,
	pool *pgxpool.Pool,
	channelID string,
	messageID int64,
	exact string,
) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	command, matched, err := ParseResearchCommand(exact)
	if err != nil || !matched {
		return err
	}
	operationID, err := NewSimulationTransitionIdentity()
	if err != nil {
		return err
	}
	preparation, err := PrepareSimulationTurnTx(ctx, tx, SimulationTurnPreparationRequest{
		OperationID: operationID, ChannelID: channelID, UserMessageID: messageID,
		InputKind: SimulationTurnExternalCommand,
	})
	if err != nil {
		return err
	}
	_, err = AuthorizeResearchPreparationTx(ctx, tx, preparation, command)
	return err
}

func persistResearchDatabaseFixture(
	ctx context.Context,
	pool *pgxpool.Pool,
	channelID string,
	messageID int64,
	exact string,
) (ResearchTurnAuthority, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ResearchTurnAuthority{}, err
	}
	defer tx.Rollback(context.Background())
	command, matched, err := ParseResearchCommand(exact)
	if err != nil || !matched {
		return ResearchTurnAuthority{}, err
	}
	operationID, err := NewSimulationTransitionIdentity()
	if err != nil {
		return ResearchTurnAuthority{}, err
	}
	preparation, err := PrepareSimulationTurnTx(ctx, tx, SimulationTurnPreparationRequest{
		OperationID: operationID, ChannelID: channelID, UserMessageID: messageID,
		InputKind: SimulationTurnExternalCommand,
	})
	if err != nil {
		return ResearchTurnAuthority{}, err
	}
	authority, err := AuthorizeResearchPreparationTx(ctx, tx, preparation, command)
	if err != nil {
		return ResearchTurnAuthority{}, err
	}
	metadata, err := json.Marshal(map[string]any{
		"channel_id": channelID, "channel_user_message_id": messageID,
		"channel_mode":                       "roleplay",
		"roleplay_simulation_preparation_id": preparation.PreparationID,
		"roleplay_world_id":                  preparation.WorldID,
		"roleplay_scene_id":                  preparation.SceneID,
		"roleplay_scene_revision":            preparation.SceneRevision,
		"roleplay_input_kind":                preparation.InputKind,
		"roleplay_participant_character_ids": preparation.ParticipantCharacterIDs,
		"roleplay_narrative_fingerprint":     preparation.NarrativeFingerprint,
		"roleplay_viewpoint_character_id":    preparation.ActiveCharacterID,
		"roleplay_generation_config":         preparation.GenerationConfig,
		"roleplay_user_turn":                 preparation.UserTurn,
	})
	if err != nil {
		return ResearchTurnAuthority{}, err
	}
	const jobID = int64(401)
	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (id,instruction,pipeline,metadata)
		VALUES ($1,$2,'chat',$3::jsonb)
	`, jobID, exact, string(metadata)); err != nil {
		return ResearchTurnAuthority{}, err
	}
	if err := BindSimulationPreparationJobTx(ctx, tx, preparation.PreparationID, jobID); err != nil {
		return ResearchTurnAuthority{}, err
	}
	if err := BindResearchPreparationJobTx(ctx, tx, preparation.PreparationID, jobID); err != nil {
		return ResearchTurnAuthority{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ResearchTurnAuthority{}, err
	}
	return authority, nil
}

func findResearchPreparationForTest(
	ctx context.Context,
	pool *pgxpool.Pool,
	preparationID string,
) (ResearchTurnAuthority, bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return ResearchTurnAuthority{}, false, err
	}
	defer tx.Rollback(context.Background())
	return loadResearchPreparationTx(ctx, tx, preparationID)
}

func installResearchTestSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE evidence (
			id BIGSERIAL PRIMARY KEY,job_id BIGINT,step_id BIGINT,kind TEXT,
			source_type TEXT,source_ref TEXT,payload_json JSONB NOT NULL,
			completion_operation_id TEXT,completion_evidence_index INTEGER
		);
		CREATE TABLE step_completion_evidence_sets (
			operation_id TEXT PRIMARY KEY,job_id BIGINT NOT NULL,
			evidence_count INTEGER NOT NULL
		);
		CREATE FUNCTION objective_completion_evidence_set_is_valid(TEXT)
		RETURNS BOOLEAN AS 'SELECT TRUE' LANGUAGE SQL;
	`); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "119_roleplay_research_authority.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(body)); err != nil {
		t.Fatalf("install research migration: %v", err)
	}
}
