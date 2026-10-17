package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

func TestLiveRedirectBindsCurrentLedgerRevisionBeforeSubmitting(t *testing.T) {
	for _, fixture := range []struct {
		name       string
		status     int
		generation int64
		revision   int64
		wantError  bool
	}{
		{"frozen execution", http.StatusOK, 1, 7, false},
		{"initial planning without ledger", http.StatusNotFound, 1, 0, false},
		{"lookup failed", http.StatusInternalServerError, 1, 0, true},
		{"generation changed", http.StatusOK, 2, 7, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var session *chatSession
			var operation model.LifecycleOperationID
			const feedback = "Keep this exact redirection; preserve café.txt."
			reads, posts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/session"):
					generation := int64(1)
					var controls []model.ChannelSessionControl
					if posts > 0 {
						generation = 2
						controls = []model.ChannelSessionControl{{OperationID: operation, JobID: 41, Generation: 2,
							Kind: model.ChannelSessionControlReplan, Text: feedback, Status: model.JobStatusRunning,
							CreatedAt: session.active.Job.CreatedAt.Add(1)}}
					}
					writeOperationGuardJSON(t, w, chatPlanReviewSnapshot(t, session, model.JobStatusRunning, generation, controls))
				case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/41/plan":
					reads++
					if r.URL.Query().Get("workspace_root") != session.channel.WorkspaceRoot || r.URL.Query().Get("workspace_identity") != session.workspaceIdentity {
						t.Error("revision lookup lost workspace authority")
					}
					if fixture.status != http.StatusOK {
						w.WriteHeader(fixture.status)
						writeOperationGuardJSON(t, w, map[string]string{"error": "ledger lookup unavailable"})
						return
					}
					writeOperationGuardJSON(t, w, redirectLedgerFixture(fixture.generation, fixture.revision))
				case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/41/replan":
					posts++
					var body struct {
						OperationID            model.LifecycleOperationID `json:"operation_id"`
						Feedback               string                     `json:"feedback"`
						ObservedLedgerRevision int64                      `json:"observed_ledger_revision"`
						WorkspaceRoot          string                     `json:"workspace_root"`
						WorkspaceIdentity      string                     `json:"workspace_identity"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					operation = body.OperationID
					if body.Feedback != feedback || body.ObservedLedgerRevision != fixture.revision || body.WorkspaceRoot != session.channel.WorkspaceRoot || body.WorkspaceIdentity != session.workspaceIdentity {
						t.Errorf("redirect lost exact authority: %+v", body)
					}
					writeOperationGuardJSON(t, w, map[string]any{"job_id": 41, "operation_id": operation, "status": model.JobStatusRunning})
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			session = newChatOperationTestSession(t, server.URL)
			session.active = waitingPlanReviewJob(t, session, 41, 1)
			session.renderer = operationGuardRenderer()
			jobID, err := session.control("redirect", feedback, true, nil)
			if (err != nil) != fixture.wantError || reads != 1 || session.planReview != nil {
				t.Fatalf("live redirect result: job=%d error=%v reads=%d review=%v", jobID, err, reads, session.planReview)
			}
			if fixture.wantError {
				if posts != 0 || session.pendingControl != nil {
					t.Fatal("failed authority lookup submitted or reserved a mutation")
				}
			} else if posts != 1 || jobID != 41 || session.pendingControl != nil || session.controls[operation].Text != feedback {
				t.Fatal("successful redirect was not reconciled with persisted control authority")
			}
		})
	}
}

func TestAmbiguousRedirectRetryRetainsOriginalOperationAndRevision(t *testing.T) {
	var session *chatSession
	var operation model.LifecycleOperationID
	reads, posts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/session"):
			writeOperationGuardJSON(t, w, chatPlanReviewSnapshot(t, session, model.JobStatusRunning, 1, nil))
		case r.URL.Path == "/v1/jobs/41/plan":
			reads++
			writeOperationGuardJSON(t, w, redirectLedgerFixture(1, 7))
		case r.URL.Path == "/v1/jobs/41/replan":
			posts++
			var body struct {
				Operation model.LifecycleOperationID `json:"operation_id"`
				Revision  int64                      `json:"observed_ledger_revision"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if posts == 1 {
				operation = body.Operation
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusConflict)
			}
			if body.Operation != operation || body.Revision != 7 {
				t.Errorf("retry changed original operation/revision: %+v", body)
			}
			writeOperationGuardJSON(t, w, map[string]string{"error": "original revision is now stale"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	session = newChatOperationTestSession(t, server.URL)
	session.active = waitingPlanReviewJob(t, session, 41, 1)
	session.renderer = operationGuardRenderer()
	if _, err := session.control("redirect", "Preserve this exact correction.", true, nil); !client.IsHTTPStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("expected ambiguous first outcome: %v", err)
	}
	if session.pendingControl == nil || session.pendingControl.observedLedgerRevision != 7 {
		t.Fatal("ambiguous redirect lost its observed revision")
	}
	if _, err := session.control("redirect", "Preserve this exact correction.", true, nil); !client.IsHTTPStatus(err, http.StatusConflict) {
		t.Fatalf("retry bypassed stale revision rejection: %v", err)
	}
	if reads != 1 || posts != 2 {
		t.Fatalf("retry refreshed authority or duplicated submission: reads=%d posts=%d", reads, posts)
	}
}

func redirectLedgerFixture(generation, revision int64) ledgerapi.CodingLedgerRevision {
	return ledgerapi.CodingLedgerRevision{LedgerID: 1, JobID: 41, Generation: generation, Revision: revision,
		State: "frozen", Ledger: model.EngineeringLedger{ID: "test-ledger", Revision: revision}}
}
