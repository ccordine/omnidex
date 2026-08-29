package api

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONMarshalsBeforeCommittingHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, map[string]any{"unsupported": make(chan int)})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != string(jsonEncodingFailure) {
		t.Fatalf("body=%q", recorder.Body.String())
	}
}

func TestWriteJSONPreservesHTMLEscapingAndEncoderNewline(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusCreated, map[string]string{"value": "<script>&"})

	if recorder.Code != http.StatusCreated || recorder.Body.String() != "{\"value\":\"\\u003cscript\\u003e\\u0026\"}\n" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

type failingJSONResponseWriter struct {
	header http.Header
	status int
}

func (w *failingJSONResponseWriter) Header() http.Header    { return w.header }
func (w *failingJSONResponseWriter) WriteHeader(status int) { w.status = status }
func (w *failingJSONResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection lost")
}

func TestWriteJSONLogsTransportFailureWithCommittedStatus(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	w := &failingJSONResponseWriter{header: make(http.Header)}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})

	if w.status != http.StatusAccepted || !strings.Contains(output.String(), "JSON response write failed status=202") ||
		!strings.Contains(output.String(), "connection lost") {
		t.Fatalf("status=%d log=%q", w.status, output.String())
	}
}
