package roleplay

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRoleplayInitiativeRotatesSuccessiveResponseRoundsWithOneAtomicTick(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, first := bootstrapRoleplayChannel(
		t, pool, "initiative-channel", "Initiative world", "First",
	)
	second, err := store.CreateCharacter(ctx, world.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.CreateCharacter(ctx, world.ID, "Third")
	if err != nil {
		t.Fatal(err)
	}
	assertSQLActiveUserInitiativeAuthority(t, pool, first.ID, second.ID, third.ID)
	for _, character := range []Character{first, second, third} {
		writeTestPersona(t, store, character.ID, character.Name+" participant.")
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Initiative scene",
		Description:    "Three participants take deterministic turns.",
		ParticipantIDs: []string{first.ID, second.ID, third.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	wantOrders := [][]string{
		{first.ID, second.ID, third.ID},
		{second.ID, third.ID, first.ID},
		{third.ID, first.ID, second.ID},
	}
	wantBefore := []SimulationInitiativeClock{
		{Round: 1, Turn: 1, FictionalTimeTick: 0},
		{Round: 1, Turn: 2, FictionalTimeTick: 1},
		{Round: 1, Turn: 3, FictionalTimeTick: 2},
	}
	wantAfter := []SimulationInitiativeClock{
		{Round: 1, Turn: 2, FictionalTimeTick: 1},
		{Round: 1, Turn: 3, FictionalTimeTick: 2},
		{Round: 2, Turn: 4, FictionalTimeTick: 3},
	}
	wantActive := []string{second.ID, third.ID, first.ID}
	revision := scene.Revision
	for index := range wantOrders {
		messageID := int64(501 + index)
		jobID := int64(601 + index)
		exact := insertNarratorRoleplayUserMessage(
			t, pool, messageID, world.ChannelID,
			"Advance deterministic initiative.", UserContributionDirection,
		)
		authority := prepareAndBindTestTurn(
			t, pool, world.ChannelID, messageID, jobID, exact,
		)
		if authority.SceneRevision != revision {
			t.Fatalf("round %d preparation revision=%d want %d", index, authority.SceneRevision, revision)
		}
		gotOrder := make([]string, len(authority.ResponderRoutes))
		for responderIndex, responder := range authority.Responders {
			gotOrder[responderIndex] = responder.CharacterID
			if responder.NarrativeProjection.Scene.Initiative != wantBefore[index] {
				t.Fatalf(
					"round %d responder %d clock=%+v want %+v",
					index, responderIndex,
					responder.NarrativeProjection.Scene.Initiative, wantBefore[index],
				)
			}
		}
		if !reflect.DeepEqual(gotOrder, wantOrders[index]) {
			t.Fatalf("round %d responders=%#v want %#v", index, gotOrder, wantOrders[index])
		}

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := MaterializeSimulationTurnTx(ctx, tx, SimulationTurnMaterializationRequest{
			PreparationID: authority.PreparationID, ChannelID: world.ChannelID,
			UserMessageID: messageID, JobID: jobID,
		}); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		advance, err := AdvanceTurnTx(ctx, tx, SimulationTurnAdvanceRequest{
			OperationID: mustTransitionID(t), PreparationID: authority.PreparationID,
			ChannelID: world.ChannelID, UserMessageID: messageID, JobID: jobID,
			ExpectedRevision: authority.SceneRevision,
		})
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if advance.BeforeInitiative != wantBefore[index] ||
			advance.AfterInitiative != wantAfter[index] ||
			advance.ActiveCharacterID != wantActive[index] {
			t.Fatalf("round %d advance=%+v", index, advance)
		}
		revision = advance.AfterRevision
	}
	persisted, err := store.ProjectCurrentScene(ctx, world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ActiveCharacterID != first.ID ||
		persisted.Initiative != wantAfter[len(wantAfter)-1] {
		t.Fatalf("persisted initiative=%+v", persisted)
	}
	directTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := directTx.Exec(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,current_character_id=$2,
		    initiative_round=$3,initiative_turn=$4,fictional_time_tick=$5,
		    updated_at=NOW()
		WHERE id=$1 AND revision=$6
	`, sceneID, second.ID, persisted.Initiative.Round, persisted.Initiative.Turn+1,
		persisted.Initiative.FictionalTimeTick+1, persisted.Revision); err != nil {
		directTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := directTx.Commit(ctx); err == nil || !strings.Contains(err.Error(), "exact authoritative turn advance") {
		t.Fatalf("unreceipted scene initiative mutation commit error=%v", err)
	}

	forgedMessageID := int64(550)
	forgedJobID := int64(650)
	forgedExact := insertNarratorRoleplayUserMessage(
		t, pool, forgedMessageID, world.ChannelID,
		"Attempt to skip initiative.", UserContributionDirection,
	)
	forgedPreparation := prepareAndBindTestTurn(
		t, pool, world.ChannelID, forgedMessageID, forgedJobID, forgedExact,
	)
	forgedAfter := SimulationInitiativeClock{
		Round: persisted.Initiative.Round, Turn: persisted.Initiative.Turn + 1,
		FictionalTimeTick: persisted.Initiative.FictionalTimeTick + 1,
	}
	forgedTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer forgedTx.Rollback(ctx)
	if _, err := forgedTx.Exec(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,current_character_id=$2,
		    initiative_round=$3,initiative_turn=$4,fictional_time_tick=$5,
		    updated_at=NOW()
		WHERE id=$1 AND revision=$6
	`, sceneID, third.ID, forgedAfter.Round, forgedAfter.Turn,
		forgedAfter.FictionalTimeTick, persisted.Revision); err != nil {
		t.Fatal(err)
	}
	forgedResult := SimulationTurnAdvanceResult{
		OperationID: mustTransitionID(t), PreparationID: forgedPreparation.PreparationID,
		WorldID: world.ID, SceneID: sceneID,
		PreviousCharacterID: first.ID, ActiveCharacterID: third.ID,
		BeforeRevision: persisted.Revision, AfterRevision: persisted.Revision + 1,
		BeforeInitiative: persisted.Initiative, AfterInitiative: forgedAfter,
		ParticipantCharacterIDs: []string{first.ID, second.ID, third.ID},
		NarrativeFingerprint:    strings.Repeat("a", 64),
		CreatedAt:               time.Now().UTC().Truncate(time.Microsecond),
	}
	forgedRequest := SimulationTurnAdvanceRequest{
		OperationID: forgedResult.OperationID, PreparationID: forgedPreparation.PreparationID,
		ChannelID: world.ChannelID, UserMessageID: forgedMessageID, JobID: forgedJobID,
		ExpectedRevision: forgedPreparation.SceneRevision,
	}
	if err := persistTurnAdvanceTx(
		ctx, forgedTx, strings.Repeat("b", 64), forgedRequest, forgedResult,
	); err == nil || !strings.Contains(err.Error(), "initiative") {
		t.Fatalf("database skipped-cursor rejection error=%v", err)
	}
}

func TestSceneCastRemovalAllowsOnlyExactFirstParticipantCursorRebase(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, first := bootstrapRoleplayChannel(
		t, pool, "initiative-cast-channel", "Cast world", "First",
	)
	second, err := store.CreateCharacter(ctx, world.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.CreateCharacter(ctx, world.ID, "Third")
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range []Character{first, second, third} {
		writeTestPersona(t, store, character.ID, character.Name+" cast participant.")
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Editable cast",
		Description:    "The active participant may leave the cast.",
		ParticipantIDs: []string{first.ID, second.ID, third.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	rebaseTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rebaseTx.Exec(ctx, `
		DELETE FROM roleplay_scene_participants WHERE scene_id=$1
	`, sceneID); err != nil {
		rebaseTx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := rebaseTx.Exec(ctx, `
		INSERT INTO roleplay_scene_participants
		    (scene_id,world_id,character_id,turn_position)
		VALUES ($1,$2,$3,0),($1,$2,$4,1)
	`, sceneID, world.ID, second.ID, third.ID); err != nil {
		rebaseTx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := rebaseTx.Exec(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,current_character_id=$2,updated_at=NOW()
		WHERE id=$1 AND revision=$3
	`, sceneID, second.ID, scene.Revision); err != nil {
		rebaseTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := rebaseTx.Commit(ctx); err != nil {
		t.Fatalf("commit exact cast-removal cursor rebase: %v", err)
	}

	forgedTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := forgedTx.Exec(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,current_character_id=$2,updated_at=NOW()
		WHERE id=$1 AND revision=$3
	`, sceneID, third.ID, scene.Revision+1); err != nil {
		forgedTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := forgedTx.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "exact authoritative turn advance") {
		t.Fatalf("forged cast cursor commit error=%v", err)
	}
}

func assertSQLActiveUserInitiativeAuthority(
	t *testing.T,
	pool interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	firstID, secondID, thirdID string,
) {
	t.Helper()
	result := map[string]any{
		"participant_character_ids": []string{firstID, secondID, thirdID},
		"active_character_id":       firstID,
		"user_turn":                 map[string]any{"persona_kind": "character", "character_id": firstID},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var responders []byte
	var next string
	if err := pool.QueryRow(context.Background(), `
		SELECT roleplay_expected_responder_ids($1::jsonb),
		       roleplay_next_initiative_character($1::jsonb)
	`, raw).Scan(&responders, &next); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(responders, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{secondID, thirdID}) || next != secondID {
		t.Fatalf("active user SQL responders=%v next=%s", got, next)
	}
	result["participant_character_ids"] = []string{firstID}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var respondersMissing, nextMissing bool
	if err := pool.QueryRow(context.Background(), `
		SELECT roleplay_expected_responder_ids($1::jsonb) IS NULL,
		       roleplay_next_initiative_character($1::jsonb) IS NULL
	`, raw).Scan(&respondersMissing, &nextMissing); err != nil {
		t.Fatal(err)
	}
	if !respondersMissing || !nextMissing {
		t.Fatal("single-participant active user unexpectedly retained a responder")
	}
}
