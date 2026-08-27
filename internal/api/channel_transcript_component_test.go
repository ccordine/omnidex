package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestChannelTranscriptComponentOwnsEscapedAccessibleMessageMarkup(t *testing.T) {
	t.Parallel()
	page := model.ChannelMessagePage{Messages: []model.ChannelMessage{
		{
			ID: 11, ChannelID: "channel-one", Role: model.ChannelMessageRoleUser,
			Content: "  <script>alert('no')</script>\nexact  ", CreatedAt: time.Date(2026, 8, 12, 12, 34, 0, 0, time.UTC),
		},
		{
			ID: 12, ChannelID: "channel-one", Role: model.ChannelMessageRoleAssistant,
			Content: "Grounded response.", CreatedAt: time.Date(2026, 8, 12, 12, 35, 0, 0, time.UTC),
		},
	}}
	response, err := channelTranscriptResponseFor("channel-one", page, false)
	if err != nil {
		t.Fatal(err)
	}
	bundle := response.HTML.Bundle
	for _, required := range []string{
		`data-recyclr-target="channel-transcript-messages"`,
		`data-recyclr-location="innerHTML"`,
		`data-channel-message-id="11"`, `data-channel-message-role="user"`,
		`aria-label="User message"`, `datetime="2026-08-12T12:34:00Z"`,
		`&lt;script&gt;alert(&#39;no&#39;)&lt;/script&gt;`,
		`data-channel-message-id="12"`, `aria-label="Assistant message"`,
	} {
		if !strings.Contains(bundle, required) {
			t.Errorf("bundle lacks %q: %s", required, bundle)
		}
	}
	if strings.Contains(bundle, "<script>") {
		t.Fatal("channel content escaped the server component boundary")
	}
}

func TestChannelTranscriptComponentNamesThePersistedRoleplaySpeaker(t *testing.T) {
	t.Parallel()
	page := model.ChannelMessagePage{Messages: []model.ChannelMessage{{
		ID: 12, ChannelID: "story", Role: model.ChannelMessageRoleAssistant,
		SpeakerName: "Mira <Harbor>", Content: "I found the signal.",
		CreatedAt: time.Date(2026, 8, 19, 20, 3, 0, 0, time.UTC),
	}}}
	response, err := channelTranscriptResponseFor("story", page, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`data-channel-message-speaker="Mira &lt;Harbor&gt;"`,
		`aria-label="Mira &lt;Harbor&gt; message"`,
		`<span>Mira &lt;Harbor&gt;</span>`,
	} {
		if !strings.Contains(response.HTML.Bundle, required) {
			t.Errorf("roleplay transcript lacks %q: %s", required, response.HTML.Bundle)
		}
	}
}

func TestChannelTranscriptComponentShowsTypedUserContributionAndRecoverableFailure(t *testing.T) {
	t.Parallel()
	page := model.ChannelMessagePage{Messages: []model.ChannelMessage{{
		ID: 13, ChannelID: "story", Role: model.ChannelMessageRoleUser,
		SpeakerName: "Gryph", Content: "[Action]\nI take Mara's hand.\n\n[Message]\nStay.",
		Roleplay: &model.ChannelMessageRoleplayAuthority{
			PersonaKind: "character", CharacterID: "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ContributionKind: "action_dialogue",
			Parts: []model.ChannelMessageRoleplayPart{
				{Kind: "action", Text: "I take Mara's hand."},
				{Kind: "message", Text: "Stay."},
			},
		},
		Turn: &model.ChannelMessageTurnState{
			JobID: 91, Status: model.JobStatusFailed, Error: `provider failed <loudly>`,
		},
		CreatedAt: time.Date(2026, 8, 20, 20, 3, 0, 0, time.UTC),
	}}}
	response, err := channelTranscriptResponseFor("story", page, false)
	if err != nil {
		t.Fatal(err)
	}
	bundle := response.HTML.Bundle
	for _, required := range []string{
		`data-channel-message-speaker="Gryph"`, `>ACTION</span>`, `I take Mara&#39;s hand.`,
		`>MESSAGE</span>`, `>Stay.</span>`,
		`role="alert"`, `data-job-id="91"`, `This turn failed.`,
		`provider failed &lt;loudly&gt;`, `data-action="chat#restoreFailedTurn"`,
		`data-roleplay-persona-kind="character"`,
		`data-roleplay-character-id="rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`data-roleplay-contribution-kind="action_dialogue"`,
		`data-roleplay-turn-parts="[{&#34;kind&#34;:&#34;action&#34;,&#34;text&#34;:&#34;I take Mara&#39;s hand.&#34;},{&#34;kind&#34;:&#34;message&#34;,&#34;text&#34;:&#34;Stay.&#34;}]"`,
		`Restore and retry`,
	} {
		if !strings.Contains(bundle, required) {
			t.Errorf("failed typed roleplay turn lacks %q: %s", required, bundle)
		}
	}
}

func TestChannelTranscriptComponentOwnsCursorAndPrependMode(t *testing.T) {
	t.Parallel()
	next := int64(31)
	page := model.ChannelMessagePage{
		Messages: []model.ChannelMessage{{
			ID: 31, ChannelID: "channel-one", Role: model.ChannelMessageRoleUser,
			Content: "Older", CreatedAt: time.Now().UTC(),
		}},
		NextBeforeID: &next,
		HasMore:      true,
	}
	response, err := channelTranscriptResponseFor("channel-one", page, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`data-recyclr-location="afterbegin"`,
		`data-recyclr-target="channel-transcript-pagination"`,
		`data-action="chat#loadOlderChannelMessages"`,
		`data-before-id="31"`, `aria-controls="channel-transcript-messages"`,
	} {
		if !strings.Contains(response.HTML.Bundle, required) {
			t.Errorf("bundle lacks %q: %s", required, response.HTML.Bundle)
		}
	}
}

func TestNeutralChatTranscriptIsOneServerRenderedActionableState(t *testing.T) {
	t.Parallel()
	component := neutralChannelTranscriptComponent()
	for _, required := range []string{
		`data-recyclr-target="channel-transcript-messages"`,
		`data-recyclr-target="channel-transcript-pagination"`,
		"Start a new conversation",
		"Sending your first message creates and selects a new conversation automatically.",
	} {
		if !strings.Contains(component.HTML.Bundle, required) {
			t.Errorf("neutral transcript lacks %q: %s", required, component.HTML.Bundle)
		}
	}
	if component.HasMore || component.NextOffset != nil {
		t.Fatalf("neutral transcript exposed pagination: %+v", component)
	}
}

func TestNeutralChatTranscriptHTTPRejectsInexactTransport(t *testing.T) {
	t.Parallel()
	server := &Server{}
	for _, test := range []struct {
		request *http.Request
		status  int
	}{
		{httptest.NewRequest(http.MethodGet, "/v1/ui/chat/neutral", nil), http.StatusOK},
		{httptest.NewRequest(http.MethodPost, "/v1/ui/chat/neutral", nil), http.StatusMethodNotAllowed},
		{httptest.NewRequest(http.MethodGet, "/v1/ui/chat/neutral?unknown=1", nil), http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		server.handleChatNeutralTranscript(response, test.request)
		if response.Code != test.status {
			t.Errorf("method=%s url=%s status=%d want=%d body=%s",
				test.request.Method, test.request.URL, response.Code, test.status, response.Body.String())
		}
	}
}

func TestChannelTranscriptComponentRejectsInvalidStoredPresentation(t *testing.T) {
	t.Parallel()
	wrongCursor := int64(9)
	fixtures := []model.ChannelMessagePage{
		{Messages: []model.ChannelMessage{{ID: 1, ChannelID: "other", Role: model.ChannelMessageRoleUser, Content: "wrong channel", CreatedAt: time.Now()}}},
		{Messages: []model.ChannelMessage{{ID: 1, ChannelID: "channel-one", Role: "tool", Content: "wrong role", CreatedAt: time.Now()}}},
		{Messages: []model.ChannelMessage{{ID: 1, ChannelID: "channel-one", Role: model.ChannelMessageRoleUser, SpeakerName: "Mira", Content: "wrong speaker", CreatedAt: time.Now()}}},
		{Messages: []model.ChannelMessage{{ID: 1, ChannelID: "channel-one", Role: model.ChannelMessageRoleAssistant, SpeakerName: " Mira", Content: "wrong speaker", CreatedAt: time.Now()}}},
		{Messages: []model.ChannelMessage{{ID: 2, ChannelID: "channel-one", Role: model.ChannelMessageRoleUser, Content: "later", CreatedAt: time.Now()}, {ID: 1, ChannelID: "channel-one", Role: model.ChannelMessageRoleUser, Content: "earlier", CreatedAt: time.Now()}}},
		{Messages: []model.ChannelMessage{{ID: 1, ChannelID: "channel-one", Role: model.ChannelMessageRoleUser, Content: "cursor mismatch", CreatedAt: time.Now()}}, NextBeforeID: &wrongCursor, HasMore: true},
	}
	for _, page := range fixtures {
		if _, err := channelTranscriptResponseFor("channel-one", page, false); err == nil {
			t.Fatalf("invalid transcript was rendered: %+v", page)
		}
	}
}

func TestChannelTranscriptQueryRejectsNoncanonicalOrDuplicateIntegers(t *testing.T) {
	t.Parallel()
	for _, rawQuery := range []string{
		"limit=01", "limit=24&limit=25", "limit=", "before_id=01",
		"before_id=4&before_id=5", "required_message_id=+4",
	} {
		request := httptest.NewRequest(http.MethodGet, "/v1/channels/authority/messages?"+rawQuery, nil)
		if strings.HasPrefix(rawQuery, "limit") {
			if _, err := exactChannelQueryInteger(request, "limit", 24, 1, 200); err == nil {
				t.Errorf("query %q was accepted", rawQuery)
			}
			continue
		}
		key := strings.SplitN(rawQuery, "=", 2)[0]
		if _, err := exactOptionalPositiveInt64Query(request, key); err == nil {
			t.Errorf("query %q was accepted", rawQuery)
		}
	}
}

func TestChannelTranscriptHTTPReturnsOnlyServerBundleAndRequiresAcceptedMessage(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	message, err := store.appendMessage("authority", model.ChannelMessageRoleUser, "Exact server message")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet,
		"/v1/channels/authority/messages?limit=24&required_message_id="+jsonNumber(message.ID), nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["messages"]; exists {
		t.Fatal("channel transcript endpoint exposed browser-renderable message records")
	}
	var rendered channelTranscriptResponse
	if err := json.Unmarshal(response.Body.Bytes(), &rendered); err != nil {
		t.Fatal(err)
	}
	if rendered.ChannelID != "authority" || !strings.Contains(rendered.HTML.Bundle, "Exact server message") {
		t.Fatalf("rendered response=%+v", rendered)
	}

	missing := httptest.NewRequest(http.MethodGet,
		"/v1/channels/authority/messages?required_message_id=999", nil)
	missingResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusConflict || !strings.Contains(missingResponse.Body.String(), "required message 999") {
		t.Fatalf("missing status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
