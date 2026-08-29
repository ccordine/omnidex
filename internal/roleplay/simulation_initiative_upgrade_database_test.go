package roleplay

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestInitiativeMigrationReconstructsLegacyCompletedMultiActorTurn(t *testing.T) {
	pool, _ := openRoleplayTestPoolWithMigrations(t, []string{
		"117_roleplay_canon_authority.sql",
		"118_roleplay_simulation_authority.sql",
		"119_roleplay_research_authority.sql",
		"120_roleplay_terminal_simulation_publication.sql",
		"122_roleplay_character_library.sql",
		"124_roleplay_character_generation_authority.sql",
		"128_roleplay_user_turn_authority.sql",
		"130_roleplay_structured_user_turns.sql",
		"131_roleplay_ordered_response_round.sql",
		"132_roleplay_response_round_publication.sql",
	})
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, first := bootstrapRoleplayChannel(
		t, pool, "initiative-upgrade", "Legacy initiative world", "First",
	)
	second, err := store.CreateCharacter(ctx, world.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range []Character{first, second} {
		writeTestPersona(t, store, character.ID, character.Name+" legacy participant.")
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	insertLegacyInitiativeScene(t, pool, world.ID, sceneID, first.ID, second.ID)

	const userMessageID int64 = 801
	exact := insertNarratorRoleplayUserMessage(
		t, pool, userMessageID, world.ChannelID,
		"Advance the completed legacy round.", UserContributionDirection,
	)
	userTurn := UserTurnAuthority{
		PersonaKind: UserPersonaNarrator, PersonaName: NarratorPersonaName,
		ContributionKind: UserContributionDirection,
		Parts:            []UserTurnPart{{Kind: UserTurnPartMessage, Text: "Advance the completed legacy round."}},
		ExactText:        exact,
	}
	preparationID := mustTransitionID(t)
	advanceID := mustTransitionID(t)
	request := SimulationTurnAdvanceRequest{
		OperationID: advanceID, PreparationID: preparationID,
		ChannelID: world.ChannelID, UserMessageID: userMessageID,
		JobID: 901, ExpectedRevision: 1,
	}
	insertLegacyCompletedTurn(t, pool, legacyInitiativeTurnFixture{
		worldID: world.ID, sceneID: sceneID, channelID: world.ChannelID,
		firstID: first.ID, secondID: second.ID, exact: exact,
		userTurn: userTurn, preparationID: preparationID, advanceRequest: request,
	})
	var legacyLifecyclePayload []byte
	if err := pool.QueryRow(ctx, `
		SELECT command_payload FROM job_lifecycle_operations WHERE operation_id=$1
	`, legacyInitiativeLifecycleID).Scan(&legacyLifecyclePayload); err != nil {
		t.Fatal(err)
	}

	migration, err := os.ReadFile(filepath.Join(
		"..", "..", "migrations", "148_roleplay_initiative_time_authority.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("upgrade legacy roleplay authority through migration 148: %v", err)
	}
	ongoingMigration, err := os.ReadFile(filepath.Join(
		"..", "..", "migrations", "149_roleplay_ongoing_action_authority.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(ongoingMigration)); err != nil {
		t.Fatalf("upgrade legacy roleplay authority through migration 149: %v", err)
	}
	var upgradedLifecyclePayload []byte
	var responseReceipts, nonNilReceipts int
	if err := pool.QueryRow(ctx, `
		SELECT operation.command_payload,
		       COUNT(*) FILTER (WHERE resolution.source_kind='response'),
		       COUNT(*) FILTER (WHERE resolution.previous_action_text IS NOT NULL OR
		                              resolution.action_text IS NOT NULL OR
		                              resolution.changed)
		FROM job_lifecycle_operations AS operation
		JOIN roleplay_ongoing_action_resolutions AS resolution
		  ON resolution.completion_operation_id=operation.operation_id
		WHERE operation.operation_id=$1
		GROUP BY operation.command_payload
	`, legacyInitiativeLifecycleID).Scan(
		&upgradedLifecyclePayload, &responseReceipts, &nonNilReceipts,
	); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(upgradedLifecyclePayload, legacyLifecyclePayload) ||
		responseReceipts != 2 || nonNilReceipts != 0 {
		t.Fatalf(
			"legacy lifecycle payload/receipts changed=%t response=%d nonnil=%d",
			!bytes.Equal(upgradedLifecyclePayload, legacyLifecyclePayload),
			responseReceipts, nonNilReceipts,
		)
	}

	var sceneActive, advanceActive, resultActive string
	var sceneRevision, sceneRound, sceneTurn, sceneTick int64
	var beforeRound, beforeTurn, beforeTick, afterRound, afterTurn, afterTick int64
	if err := pool.QueryRow(ctx, `
		SELECT scene.current_character_id,scene.revision,
		       scene.initiative_round,scene.initiative_turn,scene.fictional_time_tick,
		       advance.active_character_id,advance.result->>'active_character_id',
		       advance.before_initiative_round,advance.before_initiative_turn,
		       advance.before_fictional_time_tick,advance.after_initiative_round,
		       advance.after_initiative_turn,advance.after_fictional_time_tick
		FROM roleplay_current_scenes AS scene
		JOIN roleplay_simulation_turn_advances AS advance ON advance.scene_id=scene.id
		WHERE scene.id=$1 AND advance.operation_id=$2
	`, sceneID, advanceID).Scan(
		&sceneActive, &sceneRevision, &sceneRound, &sceneTurn, &sceneTick,
		&advanceActive, &resultActive, &beforeRound, &beforeTurn, &beforeTick,
		&afterRound, &afterTurn, &afterTick,
	); err != nil {
		t.Fatal(err)
	}
	if sceneActive != second.ID || advanceActive != second.ID || resultActive != second.ID ||
		sceneRevision != 2 ||
		[3]int64{sceneRound, sceneTurn, sceneTick} != [3]int64{1, 2, 1} ||
		[3]int64{beforeRound, beforeTurn, beforeTick} != [3]int64{1, 1, 0} ||
		[3]int64{afterRound, afterTurn, afterTick} != [3]int64{1, 2, 1} {
		t.Fatalf(
			"reconstructed scene=%s/%d/%d:%d:%d advance=%s result=%s before=%d:%d:%d after=%d:%d:%d",
			sceneActive, sceneRevision, sceneRound, sceneTurn, sceneTick,
			advanceActive, resultActive, beforeRound, beforeTurn, beforeTick,
			afterRound, afterTurn, afterTick,
		)
	}

	replayTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer replayTx.Rollback(ctx)
	replayed, err := AdvanceTurnTx(ctx, replayTx, request)
	if err != nil {
		t.Fatalf("replay reconstructed legacy advance: %v", err)
	}
	if replayed.ActiveCharacterID != second.ID ||
		replayed.BeforeInitiative != (SimulationInitiativeClock{Round: 1, Turn: 1}) ||
		replayed.AfterInitiative != (SimulationInitiativeClock{Round: 1, Turn: 2, FictionalTimeTick: 1}) {
		t.Fatalf("replayed reconstructed advance=%+v", replayed)
	}
}

func TestOngoingActionMigrationBackfillsHistoricalCharacterActionReceipt(t *testing.T) {
	pool, _ := openRoleplayTestPoolWithMigrations(t, []string{
		"117_roleplay_canon_authority.sql",
		"118_roleplay_simulation_authority.sql",
		"119_roleplay_research_authority.sql",
		"120_roleplay_terminal_simulation_publication.sql",
		"122_roleplay_character_library.sql",
		"124_roleplay_character_generation_authority.sql",
		"128_roleplay_user_turn_authority.sql",
		"130_roleplay_structured_user_turns.sql",
		"131_roleplay_ordered_response_round.sql",
		"132_roleplay_response_round_publication.sql",
	})
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, first := bootstrapRoleplayChannel(
		t, pool, "actor-action-upgrade", "Actor action world", "First",
	)
	second, err := store.CreateCharacter(ctx, world.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range []Character{first, second} {
		writeTestPersona(t, store, character.ID, character.Name+" historical participant.")
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	insertLegacyInitiativeScene(t, pool, world.ID, sceneID, first.ID, second.ID)

	const userMessageID int64 = 811
	exact, userTurn := insertHistoricalCharacterActionMessage(
		t, pool, world.ChannelID, userMessageID, first.ID, "I brace the hatch.",
	)
	preparationID := mustTransitionID(t)
	advanceID := mustTransitionID(t)
	insertLegacyCompletedTurn(t, pool, legacyInitiativeTurnFixture{
		worldID: world.ID, sceneID: sceneID, channelID: world.ChannelID,
		firstID: first.ID, secondID: second.ID, exact: exact,
		userTurn: userTurn, preparationID: preparationID,
		advanceRequest: SimulationTurnAdvanceRequest{
			OperationID: advanceID, PreparationID: preparationID,
			ChannelID: world.ChannelID, UserMessageID: userMessageID,
			JobID: 911, ExpectedRevision: 1,
		},
		advanceActiveID: second.ID,
	})
	for _, migrationName := range []string{
		"148_roleplay_initiative_time_authority.sql",
		"149_roleplay_ongoing_action_authority.sql",
	} {
		migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", migrationName))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply %s over historical actor action: %v", migrationName, err)
		}
	}

	var sourceKind, characterID string
	var sourcePosition int
	var sourceMessageID int64
	var previousStateID, currentStateID, previousAction, action *string
	var changed, terminalValid, payloadAbsent bool
	if err := pool.QueryRow(ctx, `
		SELECT resolution.source_kind,resolution.source_position,
		       resolution.character_id,resolution.source_message_id,
		       resolution.previous_state_id,resolution.current_state_id,
		       resolution.previous_action_text,resolution.action_text,resolution.changed,
		       roleplay_terminal_simulation_publication_valid($2,$3),
		       NOT operation.command_payload ? 'roleplay_user_ongoing_action'
		FROM roleplay_ongoing_action_resolutions AS resolution
		JOIN job_lifecycle_operations AS operation
		  ON operation.operation_id=resolution.completion_operation_id
		WHERE resolution.completion_operation_id=$1
		  AND resolution.source_kind='user_action'
	`, legacyInitiativeLifecycleID, preparationID, advanceID).Scan(
		&sourceKind, &sourcePosition, &characterID, &sourceMessageID,
		&previousStateID, &currentStateID, &previousAction, &action, &changed,
		&terminalValid, &payloadAbsent,
	); err != nil {
		t.Fatal(err)
	}
	if sourceKind != "user_action" || sourcePosition != -1 ||
		characterID != first.ID || sourceMessageID != userMessageID ||
		previousStateID != nil || currentStateID != nil || previousAction != nil ||
		action != nil || changed || !terminalValid || !payloadAbsent {
		t.Fatalf(
			"historical actor receipt kind/position=%s/%d character/message=%s/%d states=%v/%v actions=%v/%v changed=%t terminal=%t payloadAbsent=%t",
			sourceKind, sourcePosition, characterID, sourceMessageID,
			previousStateID, currentStateID, previousAction, action, changed,
			terminalValid, payloadAbsent,
		)
	}
}

func insertHistoricalCharacterActionMessage(
	t *testing.T,
	pool roleplayTestPool,
	channelID string,
	messageID int64,
	characterID, action string,
) (string, UserTurnAuthority) {
	t.Helper()
	request := UserTurnRequest{
		PersonaKind: UserPersonaCharacter, CharacterID: characterID,
		ContributionKind: UserContributionAction,
		Parts:            []UserTurnPart{{Kind: UserTurnPartAction, Text: action}},
	}
	exact, err := ComposeUserTurn(request)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO ai_channel_messages (id,channel_id,role,content)
		VALUES ($1,$2,'user',$3)
	`, messageID, channelID, exact); err != nil {
		t.Fatal(err)
	}
	authority, err := PersistUserTurnAuthorityTx(
		t.Context(), tx, channelID, messageID, exact, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return exact, authority
}

type legacyInitiativeTurnFixture struct {
	worldID, sceneID, channelID, firstID, secondID, exact, preparationID string
	userTurn                                                             UserTurnAuthority
	advanceRequest                                                       SimulationTurnAdvanceRequest
	advanceActiveID                                                      string
}

func insertLegacyInitiativeScene(
	t *testing.T, pool roleplayTestPool, worldID, sceneID, firstID, secondID string,
) {
	t.Helper()
	tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(context.Background(), `
		INSERT INTO roleplay_current_scenes
		    (id,world_id,title,description,current_character_id)
		VALUES ($1,$2,'Legacy scene','A completed two-character legacy scene.',$3)
	`, sceneID, worldID, firstID); err != nil {
		t.Fatal(err)
	}
	for position, characterID := range []string{firstID, secondID} {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO roleplay_scene_participants
			    (scene_id,world_id,character_id,turn_position)
			VALUES ($1,$2,$3,$4)
		`, sceneID, worldID, characterID, position); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type roleplayTestPool interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func insertLegacyCompletedTurn(t *testing.T, pool roleplayTestPool, fixture legacyInitiativeTurnFixture) {
	t.Helper()
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	firstFingerprint := strings.Repeat("a", 64)
	secondFingerprint := strings.Repeat("b", 64)
	responderIDs := []string{fixture.firstID, fixture.secondID}
	if fixture.userTurn.IsCharacter() {
		filtered := responderIDs[:0]
		for _, characterID := range responderIDs {
			if characterID != fixture.userTurn.CharacterID {
				filtered = append(filtered, characterID)
			}
		}
		responderIDs = filtered
	}
	fingerprints := []string{firstFingerprint, secondFingerprint}
	responders := make([]map[string]any, len(responderIDs))
	routes := make([]map[string]any, len(responderIDs))
	for index, characterID := range responderIDs {
		responders[index] = legacyInitiativeResponder(
			index, characterID, fixture, fingerprints[index],
		)
		routes[index] = map[string]any{
			"position": index, "character_id": characterID,
			"generation_config":     map[string]any{},
			"narrative_fingerprint": fingerprints[index],
		}
	}
	result := map[string]any{
		"preparation_id": fixture.preparationID, "channel_id": fixture.channelID,
		"user_message_id": fixture.advanceRequest.UserMessageID, "world_id": fixture.worldID,
		"scene_id": fixture.sceneID, "base_scene_revision": 1, "scene_revision": 1,
		"active_character_id": fixture.firstID, "user_turn": fixture.userTurn,
		"input_kind": "prose", "explicit_action": false,
		"participant_character_ids": []string{fixture.firstID, fixture.secondID},
		"generation_config":         map[string]any{}, "narrative_projection": responders[0]["narrative_projection"],
		"narrative_authority":   responders[0]["narrative_authority"],
		"narrative_fingerprint": firstFingerprint, "responders": responders,
		"responder_routes": routes, "created_at": createdAt,
	}
	insertLegacyCompletedTurnRows(t, pool, fixture, result, routes, responderIDs, createdAt)
}

func legacyInitiativeResponder(
	position int, characterID string, fixture legacyInitiativeTurnFixture, fingerprint string,
) map[string]any {
	return map[string]any{
		"position": position, "character_id": characterID, "generation_config": map[string]any{},
		"narrative_projection": map[string]any{"scene": map[string]any{}},
		"narrative_authority": map[string]any{
			"world_id": fixture.worldID, "scene_id": fixture.sceneID,
			"scene_revision": 1, "viewpoint_id": characterID, "fingerprint": fingerprint,
		},
		"narrative_fingerprint": fingerprint,
	}
}
