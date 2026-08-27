package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestStructuredRoleplayHTTP202ReceiptPreservesClientParts(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadAPITestMigrationBundleThroughPrefix(t, "153"),
	); err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(t.Context(), model.Channel{
		ID: "structured-http-receipt", Scope: model.ChannelScopeUser,
		Name: "Structured HTTP receipt", WorkspaceRoot: "/srv/workspaces/structured-http-receipt",
		Mode: model.ChannelModeRoleplay,
	}, "Receipt World", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(t.Context(), string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world found=%t error=%v", found, err)
	}
	actor, err := store.CreateCharacter(t.Context(), world.ID, "Gryph")
	if err != nil {
		t.Fatal(err)
	}
	for _, characterID := range []string{
		string(channel.RoleplayViewpointCharacterID), actor.ID,
	} {
		if _, err := store.WritePersona(t.Context(), roleplay.PersonaWriteRequest{
			CharacterID: characterID,
			Sheet: roleplay.PersonaSheet{
				Summary: "A participant in the receipt proof.", Voice: "Direct.",
				Traits: []string{}, Goals: []string{},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(t.Context(), roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Lantern room",
		Description: "A quiet room with one brass lantern.",
		ParticipantIDs: []string{
			string(channel.RoleplayViewpointCharacterID), actor.ID,
		},
	}); err != nil {
		t.Fatal(err)
	}

	turn := roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaCharacter, CharacterID: actor.ID,
		ContributionKind: roleplay.UserContributionStructured,
		Parts: []roleplay.UserTurnPart{
			{Kind: roleplay.UserTurnPartAction, Text: "I raise the brass lantern."},
			{Kind: roleplay.UserTurnPartEvent, Text: "Blue fire fills its glass."},
			{Kind: roleplay.UserTurnPartMessage, Text: "Keep close."},
		},
	}
	exact, err := roleplay.ComposeUserTurn(turn)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"prompt": exact, "roleplay_turn": turn,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(nil, nil)
	server.repo = repository
	server.channelStore = repository
	server.roleplaySimulation = store
	server.enqueueRoleplayChannelTurn = repository.EnqueueRoleplayChannelTurn
	server.mux = http.NewServeMux()
	server.routes()
	request := httptest.NewRequest(
		http.MethodPost, "/v1/channels/"+string(channel.ID)+"/messages", bytes.NewReader(body),
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var receipt channelMessageResponse
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	wantParts := make([]model.ChannelMessageRoleplayPart, len(turn.Parts))
	for index, part := range turn.Parts {
		wantParts[index] = model.ChannelMessageRoleplayPart{
			Kind: string(part.Kind), Text: part.Text,
		}
	}
	roleplayReceipt := receipt.UserMessage.Roleplay
	if receipt.UserMessage.Content != exact || receipt.Job.Instruction != exact ||
		roleplayReceipt == nil || roleplayReceipt.PersonaKind != "character" ||
		roleplayReceipt.CharacterID != model.RoleplayCharacterID(actor.ID) ||
		roleplayReceipt.ContributionKind != "structured_turn" ||
		!slices.Equal(roleplayReceipt.Parts, wantParts) {
		t.Fatalf("client-incompatible HTTP 202 receipt=%s", response.Body.String())
	}
}
