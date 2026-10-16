package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestChannelSessionTurnCodingModeBoundary(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		mode  codingagent.Mode
		valid bool
	}{
		{"omitted is conversation", `{"text":"exact"}`, "", true},
		{"removed normal mode", `{"text":"exact","coding_mode":"normal"}`, "", false},
		{"explicit agents", `{"text":"exact","coding_mode":"agents"}`, codingagent.Agents, true},
		{"null is not omission", `{"text":"exact","coding_mode":null}`, "", false},
		{"empty is not omission", `{"text":"exact","coding_mode":""}`, "", false},
		{"unknown mode", `{"text":"exact","coding_mode":"agent"}`, "", false},
		{"case differs", `{"text":"exact","coding_mode":"Agents"}`, "", false},
		{"duplicated mode", `{"text":"exact","coding_mode":"normal","coding_mode":"agents"}`, "", false},
		{"caller routing forbidden", `{"text":"exact","coding_mode":"agents","coding_execution":{"executor":{"provider":"other"}}}`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/channels/chat/session/turn",
				strings.NewReader(test.body))
			var decoded channelSessionTurnRequest
			err := decodeExactChannelJSON(httptest.NewRecorder(), request,
				"channel session turn request", maxChannelSessionTurnBodyBytes, &decoded)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, decode error=%v", test.valid, err)
			}
			if test.valid && (decoded.CodingMode.Value != test.mode || decoded.Text != "exact") {
				t.Fatalf("changed mode or text: %+v", decoded)
			}
		})
	}
}

func TestChannelSessionTurnOmittedModeMarshalsWithoutAuthority(t *testing.T) {
	raw, err := json.Marshal(channelSessionTurnRequest{Text: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "coding_mode") {
		t.Fatalf("omitted mode became explicit authority: %s", raw)
	}
}
