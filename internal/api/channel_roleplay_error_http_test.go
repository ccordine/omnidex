package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayChannelTurnPreparationErrorsAreNotAccepted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"unconfigured", fmt.Errorf("%w: current scene is absent", roleplay.ErrSimulationNotConfigured), http.StatusConflict},
		{"stale", fmt.Errorf("%w: scene changed", roleplay.ErrSimulationStaleRevision), http.StatusConflict},
		{"conflict", fmt.Errorf("%w: preparation identity changed", roleplay.ErrSimulationConflict), http.StatusConflict},
		{"malformed slash", fmt.Errorf("%w: quoted argument is malformed", roleplay.ErrSimulationIllegal), http.StatusBadRequest},
		{"unknown slash", fmt.Errorf("%w: command is not registered", roleplay.ErrSimulationUnknown), http.StatusBadRequest},
		{"ambiguous slash", fmt.Errorf("%w: item name is ambiguous", roleplay.ErrSimulationAmbiguous), http.StatusBadRequest},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server, store := newChannelFrontdoorTestServer(t)
			channel := store.channels["authority"]
			channel.Mode = model.ChannelModeRoleplay
			channel.RoleplayViewpointCharacterID = "rpc_0123456789abcdef0123456789abcdef"
			store.channels["authority"] = channel
			calls := 0
			server.enqueueChannelTurn = func(
				context.Context, model.ChannelID, string, string,
			) (model.ChannelMessage, model.Job, error) {
				calls++
				return model.ChannelMessage{}, model.Job{}, test.err
			}
			request := httptest.NewRequest(
				http.MethodPost, "/v1/channels/authority/messages", bytes.NewBufferString(`{"prompt":"/calibrate malformed"}`),
			)
			response := httptest.NewRecorder()

			server.Handler().ServeHTTP(response, request)

			if response.Code != test.status || calls != 1 {
				t.Fatalf("status=%d enqueue_calls=%d body=%s", response.Code, calls, response.Body.String())
			}
			if len(store.messages["authority"]) != 0 || strings.Contains(response.Body.String(), "user_message") ||
				strings.Contains(response.Body.String(), "\"job\"") {
				t.Fatalf("failed preparation exposed an accepted turn: %s", response.Body.String())
			}
		})
	}
}
