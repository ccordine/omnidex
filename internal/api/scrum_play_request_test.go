package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeScrumPlayAndPauseRequestsRequireExactRevisionBoundBodies(t *testing.T) {
	revision := "2026-08-13T12:00:00Z"
	playRecorder := httptest.NewRecorder()
	play, err := decodeScrumPlayRequest(playRecorder, httptest.NewRequest(
		"POST", "/play", strings.NewReader(`{"pivot":true,"expected_updated_at":"`+revision+`"}`),
	))
	if err != nil || !play.Pivot || play.ExpectedUpdatedAt.Value.Format("2006-01-02T15:04:05Z07:00") != revision {
		t.Fatalf("play=%+v error=%v", play, err)
	}
	pauseRecorder := httptest.NewRecorder()
	pause, err := decodeScrumPauseRequest(pauseRecorder, httptest.NewRequest(
		"POST", "/pause", strings.NewReader(`{"expected_updated_at":"`+revision+`"}`),
	))
	if err != nil || !pause.ExpectedUpdatedAt.Present {
		t.Fatalf("pause=%+v error=%v", pause, err)
	}
	for name, raw := range map[string]string{
		"missing revision":  `{"pivot":false}`,
		"unknown":           `{"pivot":false,"expected_updated_at":"` + revision + `","pipeline":"scrum"}`,
		"duplicate":         `{"pivot":false,"pivot":true,"expected_updated_at":"` + revision + `"}`,
		"trailing":          `{"pivot":false,"expected_updated_at":"` + revision + `"}{}`,
		"noncanonical time": `{"pivot":false,"expected_updated_at":"2026-08-13T12:00:00.000Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if _, err := decodeScrumPlayRequest(recorder, httptest.NewRequest("POST", "/play", strings.NewReader(raw))); err == nil {
				t.Fatal("invalid Scrum play body was accepted")
			}
		})
	}
}
