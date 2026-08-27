package queue

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCharacterTurnAllowsInitiativeCharacterAndRejectsAbsentPersona(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "persona-boundary", Scope: model.ChannelScopeUser, Name: "Persona boundary",
		WorkspaceRoot: "/srv/workspaces/persona-boundary", Mode: model.ChannelModeRoleplay,
	}, "Lantern Coast", "Responder")
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
	persona, err := store.CreateCharacter(ctx, world.ID, "Persona")
	if err != nil {
		t.Fatal(err)
	}
	absent, err := store.CreateCharacter(ctx, world.ID, "Absent")
	if err != nil {
		t.Fatal(err)
	}
	activeID := string(channel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, world.ID, activeID, persona.ID)
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
		CharacterID: absent.ID, ExpectedRevision: 0,
		Sheet: roleplay.PersonaSheet{
			Summary: "A configured character outside the current scene.", Voice: "Natural.",
			Traits: []string{}, Goals: []string{},
		},
	}); err != nil {
		t.Fatal(err)
	}

	exact, request := characterRoleplayTurn(t, absent.ID, "I answer.")
	_, _, err = repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, exact, request)
	if !errors.Is(err, roleplay.ErrSimulationIllegal) ||
		!strings.Contains(err.Error(), "selected user persona must be a current scene participant") {
		t.Fatalf("outside-scene persona error=%v", err)
	}
	assertNoPersistedRoleplayTurn(t, repository, channel.ID)
	assertDatabaseRejectsDirectRoleplayPersona(
		t, repository, channel.ID, world.ID, exact, request,
	)

	exact, request = characterRoleplayTurn(t, activeID, "I answer from the initiative cursor.")
	message, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, exact, request)
	if err != nil {
		t.Fatal(err)
	}
	var metadata channelTurnMetadata
	if json.Unmarshal(job.Metadata, &metadata) != nil {
		t.Fatal("decode active-persona responder authority")
	}
	if message.Roleplay == nil || message.Roleplay.CharacterID != model.RoleplayCharacterID(activeID) ||
		job.ID < 1 || len(metadata.RoleplayResponders) != 1 ||
		metadata.RoleplayResponders[0].CharacterID != persona.ID {
		t.Fatalf("valid persona message=%+v job=%+v", message, job)
	}
}

func TestRoleplaySoleParticipantCannotForgeCharacterTurnButNarratorRemainsUsable(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "sole-persona-boundary", Scope: model.ChannelScopeUser,
		Name: "Sole persona boundary", WorkspaceRoot: "/srv/workspaces/sole-persona-boundary",
		Mode: model.ChannelModeRoleplay,
	}, "Quiet Coast", "Only")
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
	activeID := string(channel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, world.ID, activeID)
	exact, request := characterRoleplayTurn(t, activeID, "I cannot answer myself.")
	if _, _, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, exact, request); !errors.Is(
		err, roleplay.ErrSimulationNotConfigured,
	) || !strings.Contains(err.Error(), "no responder remains") {
		t.Fatalf("sole acting persona error=%v", err)
	}
	assertNoPersistedRoleplayTurn(t, repository, channel.ID)
	if _, job, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Let the only character respond.",
	); err != nil || job.ID < 1 {
		t.Fatalf("sole-scene narrator job=%+v err=%v", job, err)
	}
}

func assertDatabaseRejectsDirectRoleplayPersona(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	worldID string,
	exact string,
	request roleplay.UserTurnRequest,
) {
	t.Helper()
	tx, err := repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var messageID int64
	if err := tx.QueryRow(t.Context(), `
		INSERT INTO ai_channel_messages (channel_id,role,content)
		VALUES ($1,'user',$2) RETURNING id
	`, channelID, exact).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	var name, summary string
	if err := tx.QueryRow(t.Context(), `
		SELECT character.name,profile.summary
		FROM roleplay_characters AS character
		JOIN roleplay_character_profiles AS profile
		  ON profile.library_character_id=character.library_character_id
		WHERE character.world_id=$1 AND character.id=$2
	`, worldID, request.CharacterID).Scan(&name, &summary); err != nil {
		t.Fatal(err)
	}
	parts, err := json.Marshal(request.Parts)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(t.Context(), `
		INSERT INTO roleplay_user_turns (
			user_message_id,channel_id,world_id,persona_kind,persona_character_id,
			persona_name,persona_summary,contribution_kind,exact_text,parts
		) VALUES ($1,$2,$3,'character',$4,$5,$6,$7,$8,$9::jsonb)
	`, messageID, channelID, worldID, request.CharacterID, name, summary,
		request.ContributionKind, exact, string(parts))
	if err == nil || !strings.Contains(
		err.Error(), "selected user persona must be a current scene participant",
	) {
		t.Fatalf("direct invalid persona insert error=%v", err)
	}
}

func characterRoleplayTurn(
	t *testing.T,
	characterID string,
	text string,
) (string, roleplay.UserTurnRequest) {
	t.Helper()
	request := roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaCharacter, CharacterID: characterID,
		ContributionKind: roleplay.UserContributionDialogue,
		Parts:            []roleplay.UserTurnPart{{Kind: roleplay.UserTurnPartMessage, Text: text}},
	}
	exact, err := roleplay.ComposeUserTurn(request)
	if err != nil {
		t.Fatal(err)
	}
	return exact, request
}

func assertNoPersistedRoleplayTurn(t *testing.T, repository *Repository, channelID model.ChannelID) {
	t.Helper()
	var messages, turns, jobs int
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT
			(SELECT COUNT(*) FROM ai_channel_messages WHERE channel_id=$1),
			(SELECT COUNT(*) FROM roleplay_user_turns WHERE channel_id=$1),
			(SELECT COUNT(*) FROM jobs WHERE metadata->>'channel_id'=$1)
	`, channelID).Scan(&messages, &turns, &jobs); err != nil {
		t.Fatal(err)
	}
	if messages != 0 || turns != 0 || jobs != 0 {
		t.Fatalf("rejected persona persisted messages=%d turns=%d jobs=%d", messages, turns, jobs)
	}
}
