package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresCancelHTTPReplaysExactlyAndReportsConflicts(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), fmt.Sprintf("cancel-http-%d", time.Now().UnixNano()),
		model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	operationID, err := queue.NewLifecycleOperationID("cancel-http", fmt.Sprintf("%d", job.ID))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository}
	first := cancelHTTPCall(t, server, job.ID, operationID, "Stop exactly once.")
	if first.Code != http.StatusOK {
		t.Fatalf("first cancellation status=%d body=%s", first.Code, first.Body.String())
	}
	exact := cancelHTTPCall(t, server, job.ID, operationID, "Stop exactly once.")
	if exact.Code != http.StatusOK || exact.Body.String() != first.Body.String() {
		t.Fatalf("exact replay status=%d body=%s want=%s", exact.Code, exact.Body.String(), first.Body.String())
	}
	changed := cancelHTTPCall(t, server, job.ID, operationID, "Changed cancellation.")
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed replay status=%d body=%s", changed.Code, changed.Body.String())
	}
	secondID, err := queue.NewLifecycleOperationID("cancel-http-second", fmt.Sprintf("%d", job.ID))
	if err != nil {
		t.Fatal(err)
	}
	distinct := cancelHTTPCall(t, server, job.ID, secondID, "Stop exactly once.")
	if distinct.Code != http.StatusConflict {
		t.Fatalf("distinct post-cancel status=%d body=%s", distinct.Code, distinct.Body.String())
	}
}

func cancelHTTPCall(
	t *testing.T,
	server *Server,
	jobID int64,
	operationID queue.LifecycleOperationID,
	reason string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"operation_id": operationID,
		"reason":       reason,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.cancelJob(response, request, jobID)
	return response
}
