package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayChannelTurnRequiresAndPreservesTypedUserAuthority(t *testing.T) {
	t.Parallel()

	server, store := newChannelFrontdoorTestServer(t)
	channel := store.channels["authority"]
	channel.Mode = model.ChannelModeRoleplay
	channel.RoleplayViewpointCharacterID = "rpc_0123456789abcdef0123456789abcdef"
	store.channels["authority"] = channel

	var captured roleplay.UserTurnRequest
	server.enqueueRoleplayChannelTurn = func(
		_ context.Context,
		channelID model.ChannelID,
		exact string,
		request roleplay.UserTurnRequest,
	) (model.ChannelMessage, model.Job, error) {
		captured = request
		message, err := store.appendMessage(channelID, model.ChannelMessageRoleUser, exact)
		message.SpeakerName = "Gryph"
		message.Roleplay = &model.ChannelMessageRoleplayAuthority{
			PersonaKind: string(request.PersonaKind), CharacterID: model.RoleplayCharacterID(request.CharacterID),
			ContributionKind: string(request.ContributionKind),
		}
		return message, model.Job{
			ID: 88, Pipeline: model.PipelineChat, Status: model.JobStatusPending,
			Instruction: exact, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}, err
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels/authority/messages",
		bytes.NewBufferString(`{"prompt":"[Message]\nHow are you?","roleplay_turn":{"persona_kind":"character","character_id":"rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","contribution_kind":"dialogue","parts":[{"kind":"message","text":"How are you?"}]}}`),
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if captured.PersonaKind != roleplay.UserPersonaCharacter ||
		captured.CharacterID != "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ||
		captured.ContributionKind != roleplay.UserContributionDialogue {
		t.Fatalf("roleplay user authority changed: %+v", captured)
	}
}

func TestRoleplayChannelTurnRejectsMissingNullOrIncompatibleUserAuthority(t *testing.T) {
	t.Parallel()

	server, store := newChannelFrontdoorTestServer(t)
	channel := store.channels["authority"]
	channel.Mode = model.ChannelModeRoleplay
	channel.RoleplayViewpointCharacterID = "rpc_0123456789abcdef0123456789abcdef"
	store.channels["authority"] = channel
	calls := 0
	server.enqueueRoleplayChannelTurn = func(
		context.Context, model.ChannelID, string, roleplay.UserTurnRequest,
	) (model.ChannelMessage, model.Job, error) {
		calls++
		return model.ChannelMessage{}, model.Job{}, nil
	}
	for _, body := range []string{
		`{"prompt":"Hello"}`,
		`{"prompt":"Hello","roleplay_turn":null}`,
		`{"prompt":"Hello","roleplay_turn":{"persona_kind":"narrator","contribution_kind":"dialogue"}}`,
		`{"prompt":"Hello","roleplay_turn":{"persona_kind":"character","character_id":"rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","contribution_kind":"narration"}}`,
		`{"prompt":"/research \"question\"","roleplay_turn":{"persona_kind":"narrator","contribution_kind":"direction"}}`,
	} {
		request := httptest.NewRequest(
			http.MethodPost, "/v1/channels/authority/messages", bytes.NewBufferString(body),
		)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
	if calls != 0 || len(store.messages["authority"]) != 0 {
		t.Fatalf("invalid roleplay authority mutated state: calls=%d messages=%d", calls, len(store.messages["authority"]))
	}
}

func TestAssistantChannelRejectsRoleplayTurnAuthority(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	calls := 0
	server.enqueueChannelTurn = func(
		context.Context, model.ChannelID, string, string,
	) (model.ChannelMessage, model.Job, error) {
		calls++
		return model.ChannelMessage{}, model.Job{}, nil
	}
	request := httptest.NewRequest(
		http.MethodPost, "/v1/channels/authority/messages",
		bytes.NewBufferString(`{"prompt":"Hello","roleplay_turn":{"persona_kind":"narrator","contribution_kind":"direction"}}`),
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || calls != 0 || len(store.messages["authority"]) != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}
