package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestSearchRoleplayContextRecordsUsesExactFrozenScopeAndPortableCharacterMemory(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	currentChannel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "roleplay-context-current", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeRoleplay, Name: "Roleplay context current",
		WorkspaceRoot: "/srv/workspaces/roleplay-context-current",
	}, "Current World", "Mira")
	if err != nil {
		t.Fatal(err)
	}
	currentWorld, found, err := store.FindWorldByChannel(ctx, string(currentChannel.ID))
	if err != nil || !found {
		t.Fatalf("resolve current roleplay world: found=%t err=%v", found, err)
	}
	currentViewpoint := roleplaySearchCharacter(t, store, currentWorld.ID, currentChannel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, currentWorld.ID, currentViewpoint.ID)
	currentScene, err := store.ProjectCurrentScene(ctx, currentWorld.ID)
	if err != nil {
		t.Fatal(err)
	}

	sourceChannel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "roleplay-context-source", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeRoleplay, Name: "Roleplay context source",
		WorkspaceRoot: "/srv/workspaces/roleplay-context-source",
	}, "Source World", "Unrelated")
	if err != nil {
		t.Fatal(err)
	}
	sourceWorld, found, err := store.FindWorldByChannel(ctx, string(sourceChannel.ID))
	if err != nil || !found {
		t.Fatalf("resolve source roleplay world: found=%t err=%v", found, err)
	}
	unrelatedCharacter := roleplaySearchCharacter(
		t, store, sourceWorld.ID, sourceChannel.RoleplayViewpointCharacterID,
	)
	portablePlacement, err := store.PlaceLibraryCharacter(
		ctx, sourceWorld.ID, currentViewpoint.LibraryID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
		CharacterID: unrelatedCharacter.ID, ExpectedRevision: 0,
		Sheet: roleplay.PersonaSheet{
			Summary: "An unrelated source-world participant.", Voice: "Natural.",
			Traits: []string{}, Goals: []string{},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sourceSceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
		ID: sourceSceneID, WorldID: sourceWorld.ID, Title: "Source scene",
		Description:    "A separate scene used to prove exact search scope.",
		ParticipantIDs: []string{unrelatedCharacter.ID, portablePlacement.ID},
	}); err != nil {
		t.Fatal(err)
	}
	sourceScene, err := store.ProjectCurrentScene(ctx, sourceWorld.ID)
	if err != nil {
		t.Fatal(err)
	}

	visibleCanon := "The cobalt compass opens the current-world archive."
	currentSourceMessageID := appendRoleplaySearchAssistantMessage(
		t, repository, currentChannel.ID, "A cobalt compass turns in the archive door.",
	)
	currentEvent, err := store.AppendCanonEvent(
		ctx, currentWorld.ID, currentSourceMessageID, visibleCanon,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.GrantKnowledge(ctx, currentViewpoint.ID, currentEvent.ID); err != nil || !created {
		t.Fatalf("grant current-world visible canon: created=%t err=%v", created, err)
	}

	portableMemory := "I remember aligning the cobalt compass in Source World."
	unrelatedMemory := "The unrelated character remembers hiding a cobalt compass."
	sourceMessageID := appendRoleplaySearchAssistantMessage(
		t, repository, sourceChannel.ID, "Two travelers discuss a cobalt compass.",
	)
	sourceEvent, err := store.AppendCanonEvent(
		ctx, sourceWorld.ID, sourceMessageID, "A cobalt compass was aligned in Source World.",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, characterID := range []string{portablePlacement.ID, unrelatedCharacter.ID} {
		if _, created, err := store.GrantKnowledge(ctx, characterID, sourceEvent.ID); err != nil || !created {
			t.Fatalf("grant source-world knowledge to %s: created=%t err=%v", characterID, created, err)
		}
	}
	if _, err := store.AppendCharacterMemory(
		ctx, portablePlacement.ID, sourceEvent.ID, portableMemory,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterMemory(
		ctx, unrelatedCharacter.ID, sourceEvent.ID, unrelatedMemory,
	); err != nil {
		t.Fatal(err)
	}

	frozenAt := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
	advanceRoleplaySearchScene(t, repository, currentScene.ID)
	advanceRoleplaySearchScene(t, repository, sourceScene.ID)
	includedEvent := "The cobalt compass chimed in the frozen current scene."
	futureEvent := "The cobalt compass chimed after the frozen time."
	foreignSceneEvent := "The cobalt compass chimed in another world's scene."
	appendRoleplaySearchTransition(
		t, repository, currentWorld.ID, currentScene.ID, currentViewpoint.ID,
		frozenAt.Add(-2*time.Minute), includedEvent,
	)
	appendRoleplaySearchTransition(
		t, repository, currentWorld.ID, currentScene.ID, currentViewpoint.ID,
		frozenAt.Add(time.Minute), futureEvent,
	)
	appendRoleplaySearchTransition(
		t, repository, sourceWorld.ID, sourceScene.ID, unrelatedCharacter.ID,
		frozenAt.Add(-2*time.Minute), foreignSceneEvent,
	)

	records, err := repository.SearchRoleplayContextRecords(
		ctx, currentWorld.ID, currentChannel.RoleplayViewpointCharacterID,
		currentScene.ID, frozenAt, []string{"cobalt compass"}, 16,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"fictional_canon":  visibleCanon,
		"character_memory": portableMemory,
		"simulation_event": includedEvent,
	}
	if len(records) != len(want) {
		t.Fatalf("roleplay context records=%#v, want exactly %#v", records, want)
	}
	seen := make(map[string]string, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.SourceID) == "" {
			t.Fatalf("roleplay context record omitted exact source identity: %#v", record)
		}
		if previous, exists := seen[record.Namespace]; exists {
			t.Fatalf("duplicate namespace %q contents %q and %q", record.Namespace, previous, record.Content)
		}
		seen[record.Namespace] = record.Content
	}
	for namespace, content := range want {
		if seen[namespace] != content {
			t.Fatalf("roleplay context namespace %q=%q, want %q; all=%#v", namespace, seen[namespace], content, records)
		}
	}
	for _, excluded := range []string{unrelatedMemory, futureEvent, foreignSceneEvent} {
		for _, record := range records {
			if record.Content == excluded {
				t.Fatalf("out-of-scope roleplay authority leaked into frozen search: %#v", record)
			}
		}
	}
}

func roleplaySearchCharacter(
	t *testing.T,
	store *roleplay.Store,
	worldID string,
	characterID model.RoleplayCharacterID,
) roleplay.Character {
	t.Helper()
	characters, err := store.ListCharacters(t.Context(), worldID)
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range characters {
		if character.ID == string(characterID) {
			return character
		}
	}
	t.Fatalf("roleplay world %s omitted character %s", worldID, characterID)
	return roleplay.Character{}
}

func appendRoleplaySearchAssistantMessage(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	content string,
) int64 {
	t.Helper()
	var messageID int64
	if err := repository.pool.QueryRow(t.Context(), `
		INSERT INTO ai_channel_messages (channel_id,role,content)
		VALUES ($1,'assistant',$2)
		RETURNING id
	`, channelID, content).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	return messageID
}

func advanceRoleplaySearchScene(
	t *testing.T,
	repository *Repository,
	sceneID string,
) {
	t.Helper()
	if _, err := repository.pool.Exec(t.Context(), `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,updated_at=NOW()
		WHERE id=$1
	`, sceneID); err != nil {
		t.Fatal(err)
	}
}

func appendRoleplaySearchTransition(
	t *testing.T,
	repository *Repository,
	worldID, sceneID, actorCharacterID string,
	createdAt time.Time,
	narrativeEvent string,
) {
	t.Helper()
	operationID, err := roleplay.NewSimulationTransitionIdentity()
	if err != nil {
		t.Fatal(err)
	}
	result := roleplay.SimulationTransitionResult{
		Schema:      roleplay.SimulationTransitionSchemaV1,
		OperationID: operationID, WorldID: worldID, SceneID: sceneID,
		ActorCharacterID: actorCharacterID, BeforeRevision: 1, AfterRevision: 2,
		Action:          roleplay.SimulationAction{Kind: roleplay.SimulationActionAutomatic},
		Effects:         []roleplay.SimulationEffect{{Sequence: 1, Kind: "context_search_fixture"}},
		NarrativeEvents: []string{narrativeEvent}, CreatedAt: createdAt,
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var observerPayload []byte
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT jsonb_agg(character_id ORDER BY turn_position,character_id)
		FROM roleplay_scene_participants
		WHERE world_id=$1 AND scene_id=$2
	`, worldID, sceneID).Scan(&observerPayload); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(t.Context(), `
		INSERT INTO roleplay_simulation_transitions (
			operation_id,world_id,scene_id,actor_character_id,
			before_revision,after_revision,exact_action,action_kind,command_key,
			request_sha256,result,observer_character_ids,created_at
		) VALUES ($1,$2,$3,$4,1,2,'','automatic','',$5,$6::jsonb,$7::jsonb,$8)
	`, operationID, worldID, sceneID, actorCharacterID,
		strings.Repeat("0", 64), payload, observerPayload, createdAt); err != nil {
		t.Fatal(err)
	}
}
