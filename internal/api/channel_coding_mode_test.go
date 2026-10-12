package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestChannelMessageCodingModeBoundary(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		mode  codingagent.Mode
		valid bool
	}{
		{"omitted preserves normal", `{"prompt":"exact"}`, "", true},
		{"explicit normal", `{"prompt":"exact","coding_mode":"normal"}`, codingagent.Normal, true},
		{"explicit agents", `{"prompt":"exact","coding_mode":"agents"}`, codingagent.Agents, true},
		{"null is not omission", `{"prompt":"exact","coding_mode":null}`, "", false},
		{"empty is not omission", `{"prompt":"exact","coding_mode":""}`, "", false},
		{"unknown mode", `{"prompt":"exact","coding_mode":"agent"}`, "", false},
		{"case differs", `{"prompt":"exact","coding_mode":"Agents"}`, "", false},
		{"duplicated mode", `{"prompt":"exact","coding_mode":"normal","coding_mode":"agents"}`, "", false},
		{"caller routing forbidden", `{"prompt":"exact","coding_mode":"agents","coding_execution":{"executor":{"provider":"other"}}}`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/channels/chat/messages", strings.NewReader(test.body))
			var decoded channelMessageRequest
			err := decodeExactChannelJSON(httptest.NewRecorder(), request, "channel message request", maxChannelMessageBodyBytes, &decoded)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, decode error=%v", test.valid, err)
			}
			if test.valid && (decoded.CodingMode.Value != test.mode || decoded.Prompt != "exact") {
				t.Fatalf("changed mode or prompt: %+v", decoded)
			}
		})
	}
}
