package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestChatSlashCommandsRenderOnlyEscapedServerOptionsAndCursorAuthority(t *testing.T) {
	t.Parallel()
	channel := slashCommandTestChannel(model.ChannelModeRoleplay)
	projection := &roleplay.SimulationSlashCommandProjection{
		Schema:  roleplay.SimulationSlashCommandProjectionSchemaV1,
		WorldID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SceneID: "rps_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SceneRevision: 7,
		ActiveCharacterID: "rpc_cccccccccccccccccccccccccccccccc", ActiveCharacterName: "Archivist",
		Commands: []roleplay.SimulationSlashCommand{
			{
				ID: "slash-command-option-0123456789abcdef", Kind: roleplay.SimulationSlashCommandInteraction,
				Key: "offer", Insertion: `/offer ""`, Display: `/offer "…"`, Label: `Offer <signal>`,
				Description: `Offer a <signal> to the active character.`, CursorUTF16: 8, DisplayOrder: 0,
			},
			{
				ID: "slash-command-option-fedcba9876543210", Kind: roleplay.SimulationSlashCommandResearch,
				Key: "research", Insertion: `/research ""`, Display: `/research "…"`, Label: "Research the web",
				Description: "Research a real-world question as Archivist.", CursorUTF16: 11, DisplayOrder: 1,
			},
		},
	}
	component, err := renderChatSlashCommandsComponent(channel, projection)
	if err != nil {
		t.Fatal(err)
	}
	if component.ChannelID != channel.ID || component.CommandCount != 2 {
		t.Fatalf("component=%+v", component)
	}
	for _, expected := range []string{
		`data-recyclr-target="slash-command-options"`, `id="slash-command-list"`,
		`data-slash-command-channel-id="slash-test"`, `role="listbox"`,
		`id="slash-command-option-0123456789abcdef"`, `data-chat-slash-option`,
		`data-action="chat#chooseSlashCommand"`, `data-slash-command="/offer &#34;&#34;"`,
		`data-slash-command-cursor="8"`, `data-slash-command-prefix="/offer"`,
		`data-slash-command-kind="interaction"`, `data-slash-command-key="offer"`,
		`data-slash-command-order="0"`, `/offer &#34;…&#34;`, `Offer &lt;signal&gt;`,
		`data-slash-command-no-match role="status" hidden class="rounded-md`,
	} {
		if !strings.Contains(component.HTML.Bundle, expected) {
			t.Errorf("slash command component lacks %q: %s", expected, component.HTML.Bundle)
		}
	}
	for _, forbidden := range []string{"<signal>", "tool", "operation"} {
		if strings.Contains(component.HTML.Bundle, forbidden) {
			t.Errorf("slash command component leaked forbidden %q: %s", forbidden, component.HTML.Bundle)
		}
	}
}

func TestAssistantSlashCommandComponentIsExplicitlyEmpty(t *testing.T) {
	t.Parallel()
	component, err := renderChatSlashCommandsComponent(slashCommandTestChannel(model.ChannelModeAssistant), nil)
	if err != nil {
		t.Fatal(err)
	}
	if component.CommandCount != 0 || !strings.Contains(
		component.HTML.Bundle, "No slash commands are available for this assistant conversation.",
	) || !strings.Contains(component.HTML.Bundle, `data-slash-command-no-match role="status" class="rounded-md`) {
		t.Fatalf("component=%+v", component)
	}
	if strings.Contains(component.HTML.Bundle, `data-chat-slash-option`) {
		t.Fatalf("assistant component exposed a slash option: %s", component.HTML.Bundle)
	}
}

func TestChatSlashCommandComponentRejectsMalformedProjection(t *testing.T) {
	t.Parallel()
	base := roleplay.SimulationSlashCommandProjection{
		Schema:  roleplay.SimulationSlashCommandProjectionSchemaV1,
		WorldID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SceneID: "rps_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SceneRevision: 1,
		ActiveCharacterID: "rpc_cccccccccccccccccccccccccccccccc", ActiveCharacterName: "Archivist",
		Commands: []roleplay.SimulationSlashCommand{{
			ID: "slash-command-option-0123456789abcdef", Kind: roleplay.SimulationSlashCommandInteraction,
			Key: "wave", Insertion: "/wave", Display: "/wave", Label: "Wave", Description: "Wave.",
			CursorUTF16: 5, DisplayOrder: 0,
		}},
	}
	tests := []func(*roleplay.SimulationSlashCommandProjection){
		func(value *roleplay.SimulationSlashCommandProjection) { value.Schema = "invalid" },
		func(value *roleplay.SimulationSlashCommandProjection) { value.Commands[0].ID = "browser-id" },
		func(value *roleplay.SimulationSlashCommandProjection) { value.Commands[0].DisplayOrder = 2 },
		func(value *roleplay.SimulationSlashCommandProjection) { value.Commands[0].CursorUTF16 = 99 },
		func(value *roleplay.SimulationSlashCommandProjection) { value.Commands[0].Insertion = "/other" },
		func(value *roleplay.SimulationSlashCommandProjection) {
			value.Commands[0].Kind = roleplay.SimulationSlashCommandGive
		},
		func(value *roleplay.SimulationSlashCommandProjection) {
			value.Commands[0].Key = "research"
			value.Commands[0].Insertion = `/research ""`
			value.Commands[0].Display = `/research "…"`
			value.Commands[0].CursorUTF16 = 11
		},
		func(value *roleplay.SimulationSlashCommandProjection) {
			value.Commands[0].Kind = roleplay.SimulationSlashCommandTake
			value.Commands[0].Key = "take"
			value.Commands[0].Insertion = `/take "🍔"`
			value.Commands[0].Display = `/take "🍔"`
			value.Commands[0].CursorUTF16 = 8
		},
	}
	for _, mutate := range tests {
		candidate := base
		candidate.Commands = append([]roleplay.SimulationSlashCommand(nil), base.Commands...)
		mutate(&candidate)
		if _, err := renderChatSlashCommandsComponent(slashCommandTestChannel(model.ChannelModeRoleplay), &candidate); err == nil {
			t.Fatalf("malformed projection was rendered: %+v", candidate)
		}
	}
}

func TestChatSlashCommandsHTTPUsesSelectedChannelAuthority(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	simulation.slashCommands = &roleplay.SimulationSlashCommandProjection{
		Schema:  roleplay.SimulationSlashCommandProjectionSchemaV1,
		WorldID: simulation.world.ID, SceneID: simulation.scene.ID, SceneRevision: simulation.scene.Revision,
		ActiveCharacterID: simulation.scene.ActiveCharacterID, ActiveCharacterName: "Active Archivist",
		Commands: []roleplay.SimulationSlashCommand{{
			ID: "slash-command-option-0123456789abcdef", Kind: roleplay.SimulationSlashCommandTake,
			Key: "take", Insertion: `/take "Compass"`, Display: `/take "Compass"`, Label: "Take Compass",
			Description: "Take Compass from Active Archivist.", CursorUTF16: 15, DisplayOrder: 0,
		}},
	}
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/chat/slash-commands?channel_id=story-http", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || simulation.slashCommandCalls != 1 ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, simulation.slashCommandCalls, response.Header(), response.Body.String())
	}
	var payload chatSlashCommandsComponent
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChannelID != roleplayHTTPChannelID || payload.CommandCount != 1 ||
		!strings.Contains(payload.HTML.Bundle, `/take &#34;Compass&#34;`) {
		t.Fatalf("payload=%+v body=%s", payload, response.Body.String())
	}
}

func TestChatSlashCommandsHTTPReturnsAssistantEmptyWithoutSimulationStore(t *testing.T) {
	t.Parallel()
	channels := newChannelTestStore()
	channel := slashCommandTestChannel(model.ChannelModeAssistant)
	channels.channels[string(channel.ID)] = channel
	server := NewServerWithOptions(nil, nil, ServerOptions{})
	server.channelStore = channels
	server.mux = http.NewServeMux()
	server.routes()
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/v1/ui/chat/slash-commands?channel_id=slash-test", nil,
	))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "assistant conversation") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestChatSlashCommandsHTTPReturnsExplicitConflictWhenRoleplaySceneIsMissing(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	server := newRoleplayHTTPTestServer(t, simulation)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/v1/ui/chat/slash-commands?channel_id=story-http", nil,
	))
	if response.Code != http.StatusConflict || simulation.slashCommandCalls != 1 ||
		!strings.Contains(response.Body.String(), roleplay.ErrSimulationNotConfigured.Error()) {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, simulation.slashCommandCalls, response.Body.String())
	}
}

func TestChatSlashCommandsHTTPRejectsInexactTransport(t *testing.T) {
	t.Parallel()
	server := NewServerWithOptions(nil, nil, ServerOptions{})
	server.mux = http.NewServeMux()
	server.routes()
	tests := []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPost, "/v1/ui/chat/slash-commands?channel_id=slash-test", http.StatusMethodNotAllowed},
		{http.MethodGet, "/v1/ui/chat/slash-commands", http.StatusBadRequest},
		{http.MethodGet, "/v1/ui/chat/slash-commands?channel_id=slash-test&offset=0", http.StatusBadRequest},
		{http.MethodGet, "/v1/ui/chat/slash-commands?channel_id=slash-test&channel_id=other", http.StatusBadRequest},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.status {
			t.Errorf("method=%s path=%s status=%d want=%d body=%s", test.method, test.path, response.Code, test.status, response.Body.String())
		}
	}
}

func slashCommandTestChannel(mode model.ChannelMode) model.Channel {
	now := time.Now().UTC()
	channel := model.Channel{
		ID: "slash-test", Scope: model.ChannelScopeUser, Name: "Slash test", Tags: []string{"user-channel"},
		ProjectID: 42, WorkspaceRoot: "/workspace/slash-test", Mode: mode, CreatedAt: now, UpdatedAt: now,
	}
	if mode == model.ChannelModeRoleplay {
		channel.RoleplayViewpointCharacterID = "rpc_cccccccccccccccccccccccccccccccc"
	}
	return channel
}
