package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

type panelControlBody struct {
	OperationID, OwnerTurnID, OwnerTurnText string
	Revision                                int64
	WorkspaceRoot, WorkspaceIdentity        string
	Action                                  client.DirectPlanningAction
}

// Decode exact transport names independently of the production client.
func decodePanelControl(t *testing.T, r *http.Request) panelControlBody {
	t.Helper()
	var raw struct {
		OperationID       string                      `json:"operation_id"`
		OwnerTurnID       string                      `json:"owner_turn_id"`
		OwnerTurnText     string                      `json:"owner_turn_text"`
		Revision          int64                       `json:"revision"`
		WorkspaceRoot     string                      `json:"workspace_root"`
		WorkspaceIdentity string                      `json:"workspace_identity"`
		Action            client.DirectPlanningAction `json:"action"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		t.Error(err)
	}
	return panelControlBody{raw.OperationID, raw.OwnerTurnID, raw.OwnerTurnText, raw.Revision, raw.WorkspaceRoot, raw.WorkspaceIdentity, raw.Action}
}

func panelSession(t *testing.T, url string) *chatSession {
	t.Helper()
	session := newChatOperationTestSession(t, url)
	view, _ := testWorkspace(t, 100, 24)
	session.renderer = chatRenderer{console: &chatConsole{workspace: view}}
	session.active = &model.JobDetails{Job: model.Job{ID: 801, CurrentGeneration: 1}}
	view.ledger.opened = true
	view.focus = workspaceLedgerPane
	return session
}

func TestLedgerPanelAuthoritativeReadControlsAndReconciliation(t *testing.T) {
	snapshot := panelLedgerFixture()
	reads, mutations := 0, 0
	var session *chatSession
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads++
			if r.URL.Path != "/v1/jobs/801/plan" || r.URL.Query().Get("workspace_root") != session.channel.WorkspaceRoot || r.URL.Query().Get("workspace_identity") != session.workspaceIdentity {
				t.Error("read lost workspace authority")
			}
			writeChatPlanReviewJSON(t, w, 200, snapshot)
			return
		}
		mutations++
		if r.URL.Path != "/v1/jobs/801/plan/decisions" {
			t.Error("wrong mutation path")
		}
		body := decodePanelControl(t, r)
		if body.Revision != snapshot.Revision || body.WorkspaceRoot != session.channel.WorkspaceRoot || body.WorkspaceIdentity != session.workspaceIdentity || body.OwnerTurnID != "terminal-"+body.OperationID {
			t.Errorf("mutation authority: %+v", body)
		}
		// The fixture simulates a server additionally changing rationale. The client
		// must render that returned value rather than patching its old projection.
		snapshot.BaseRevision = snapshot.Revision
		snapshot.Revision++
		snapshot.Ledger.Revision = snapshot.Revision
		snapshot.OperationID = body.OperationID
		task := &snapshot.Ledger.Tasks[1]
		switch body.Action.Kind {
		case model.LedgerMutationReview:
			task.ReviewState = body.Action.ReviewState
		case model.LedgerMutationRemove:
			task.Disposition = model.LedgerDispositionTombstoned
			task.ReviewState = ""
		case model.LedgerMutationRestore:
			task.Disposition = model.LedgerDispositionActive
			task.ReviewState = model.LedgerReviewProposed
		case model.LedgerMutationReorder:
			if body.Action.Rank != nil {
				task.Rank = *body.Action.Rank
			} else {
				task.Rank += int64(body.Action.Move)
			}
		default:
			t.Errorf("unexpected control %q", body.Action.Kind)
		}
		task.Rationale = "authoritative server rationale"
		writeChatPlanReviewJSON(t, w, 200, snapshot)
	}))
	defer server.Close()
	session = panelSession(t, server.URL)
	if err := session.refreshLedgerPanel(); err != nil {
		t.Fatal(err)
	}
	view := session.ledgerWorkspace()
	view.ledger.selected = "panel-second"
	for _, key := range []byte{'a', 'd', 'x', 's', ']', '['} {
		if err := view.key(key, false); err != nil {
			t.Fatal(err)
		}
		request := <-view.ledger.requests
		if err := session.acceptLedgerPanelRequest(request); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(view.ledger.snapshot.Ledger, snapshot.Ledger) || view.ledger.selected != "panel-second" || session.pendingPlan != nil {
			t.Fatal("did not reconcile server-confirmed state")
		}
	}
	if reads != 1 || mutations != 6 {
		t.Fatalf("reads=%d mutations=%d", reads, mutations)
	}
}

func TestLedgerPanelConflictPermissionAndReadFailure(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			snapshot := panelLedgerFixture()
			snapshot.Revision = 4
			snapshot.Ledger.Revision = 4
			reads, mutations := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					reads++
					writeChatPlanReviewJSON(t, w, 200, snapshot)
					return
				}
				mutations++
				body := decodePanelControl(t, r)
				if body.Revision != 3 {
					t.Error("stale operation silently rebased")
				}
				writeChatPlanReviewJSON(t, w, status, map[string]string{"error": "revision or workspace permission rejected"})
			}))
			defer server.Close()
			session := panelSession(t, server.URL)
			view := session.ledgerWorkspace()
			if err := view.acceptLedger(panelLedgerFixture()); err != nil {
				t.Fatal(err)
			}
			if err := view.key('x', false); err != nil {
				t.Fatal(err)
			}
			if err := session.acceptLedgerPanelRequest(<-view.ledger.requests); err != nil {
				t.Fatal(err)
			}
			if status == http.StatusForbidden {
				// The shared client does not classify an arbitrary 403 as a
				// definitive pre-commit rejection. Preserve its retry authority.
				if mutations != 1 || reads != 0 || session.pendingPlan == nil || view.ledger.snapshot.Revision != 3 || !strings.Contains(view.ledger.feedback, "permission rejected") || !strings.Contains(view.ledger.feedback, "outcome unresolved") {
					t.Fatalf("permission response lost existing retry rules: %+v pending=%+v", view.ledger, session.pendingPlan)
				}
			} else if mutations != 1 || reads != 1 || session.pendingPlan != nil || view.ledger.snapshot.Revision != 4 || !strings.Contains(view.ledger.feedback, "rejected") {
				t.Fatalf("rejection state: %+v pending=%+v", view.ledger, session.pendingPlan)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatPlanReviewJSON(t, w, 503, map[string]string{"error": "fixture unavailable"})
	}))
	defer server.Close()
	session := panelSession(t, server.URL)
	if err := session.refreshLedgerPanel(); err != nil {
		t.Fatal(err)
	}
	if session.ledgerWorkspace().ledger.loading || !strings.Contains(session.ledgerWorkspace().ledger.feedback, "refresh failed") {
		t.Fatal("failed read stuck loading")
	}
}

func TestLedgerPanelAmbiguousRetryRetainsExactPayloadAndBlocksOtherActions(t *testing.T) {
	snapshot := panelLedgerFixture()
	var first panelControlBody
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeChatPlanReviewJSON(t, w, 200, snapshot)
			return
		}
		body := decodePanelControl(t, r)
		mutations++
		if mutations == 1 {
			first = body
			dropOperationGuardConnection(t, w)
			return
		}
		if !reflect.DeepEqual(first, body) {
			t.Errorf("retry changed payload: %+v / %+v", first, body)
		}
		snapshot.BaseRevision = 3
		snapshot.Revision = 4
		snapshot.Ledger.Revision = 4
		snapshot.OperationID = body.OperationID
		snapshot.Ledger.Tasks[0].ReviewState = model.LedgerReviewAgreed
		writeChatPlanReviewJSON(t, w, 200, snapshot)
	}))
	defer server.Close()
	session := panelSession(t, server.URL)
	view := session.ledgerWorkspace()
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := view.key('a', false); err != nil {
		t.Fatal(err)
	}
	if err := session.acceptLedgerPanelRequest(<-view.ledger.requests); err != nil {
		t.Fatal(err)
	}
	if session.pendingPlan == nil || view.ledger.snapshot.Revision != 3 || !strings.Contains(view.ledger.feedback, "unresolved") {
		t.Fatal("lost ambiguous operation")
	}
	changed := workspaceLedgerRequest{key: 'x', jobID: 801, generation: 1, revision: 3, action: client.DirectPlanningAction{Kind: model.LedgerMutationRemove, TargetID: "panel-first"}}
	if err := session.acceptLedgerPanelRequest(changed); err != nil {
		t.Fatal(err)
	}
	if mutations != 1 {
		t.Fatal("dispatched competing operation")
	}
	if err := session.acceptLedgerPanelRequest(workspaceLedgerRequest{key: 't'}); err != nil {
		t.Fatal(err)
	}
	if mutations != 2 || session.pendingPlan != nil || view.ledger.snapshot.Revision != 4 {
		t.Fatal("same operation retry did not reconcile")
	}
}

func TestLedgerPanelRejectsForeignAndMalformedResponses(t *testing.T) {
	for _, change := range []func(*ledgerapi.CodingLedgerRevision){
		func(r *ledgerapi.CodingLedgerRevision) { r.JobID++ },
		func(r *ledgerapi.CodingLedgerRevision) { r.Generation++ },
		func(r *ledgerapi.CodingLedgerRevision) { r.OperationID = "different-operation" },
		func(r *ledgerapi.CodingLedgerRevision) { r.Ledger.Revision++ },
	} {
		snapshot := panelLedgerFixture()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodePanelControl(t, r)
			snapshot.BaseRevision = 3
			snapshot.Revision = 4
			snapshot.Ledger.Revision = 4
			snapshot.OperationID = body.OperationID
			change(&snapshot)
			writeChatPlanReviewJSON(t, w, 200, snapshot)
		}))
		session := panelSession(t, server.URL)
		view := session.ledgerWorkspace()
		if err := view.acceptLedger(panelLedgerFixture()); err != nil {
			t.Fatal(err)
		}
		if err := view.key('a', false); err != nil {
			t.Fatal(err)
		}
		if err := session.acceptLedgerPanelRequest(<-view.ledger.requests); err != nil {
			t.Fatal(err)
		}
		if session.pendingPlan == nil || view.ledger.snapshot.Revision != 3 {
			t.Fatal("malformed success accepted")
		}
		server.Close()
	}
}

func TestTaskPacketApprovalAndLedgerDetailPresentation(t *testing.T) {
	snapshot := panelLedgerFixture()
	snapshot.CreatedAt = time.Now().UTC()
	snapshot.UpdatedAt = snapshot.CreatedAt
	task := &snapshot.Ledger.Tasks[0]
	task.Objective = "Provide a separate Research destination."
	task.OwnerContext = []model.TaskContext{{Text: "Read-only research is approved.", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner-17", Revision: 2}}}
	task.Constraints = []string{"Preserve accepted work."}
	task.FileResponsibilities = []model.TaskFileResponsibility{{Path: task.WriteFiles[0], Action: "change", Responsibility: "Preserve routing."}}
	task.Acceptance = []model.TaskAcceptance{{ID: "research", Text: "Research remains separate.", Verification: []string{"Run routing and read-only checks."}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeChatPlanReviewJSON(t, w, 200, snapshot) }))
	defer server.Close()
	session := panelSession(t, server.URL)
	plan, err := session.client.CodingPlan(session.ctx, session.channel, session.workspaceIdentity, snapshot.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Leaves[0].Task == nil || !reflect.DeepEqual(*plan.Leaves[0].Task, *task) {
		t.Fatal("terminal transport discarded approved task fields")
	}
	rendered, err := renderPlanReview(mustPlanReviewState(t, plan))
	if err != nil {
		t.Fatal(err)
	}
	if err = session.refreshLedgerPanel(); err != nil {
		t.Fatal(err)
	}
	view := session.ledgerWorkspace()
	view.mu.Lock()
	details := strings.Join(view.ledgerLinesLocked(100), "\n")
	view.mu.Unlock()
	for _, text := range []string{task.Objective, task.OwnerContext[0].Text, "owner-17", task.Constraints[0], task.FileResponsibilities[0].Responsibility, task.Acceptance[0].Text, task.Acceptance[0].Verification[0]} {
		if !strings.Contains(rendered, text) || !strings.Contains(details, text) {
			t.Fatal("approval or ledger details lost packet field", text)
		}
	}
}

func TestLedgerPanelSeparatesPendingAmendmentsFromFrozenItems(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	view.focus = workspaceLedgerPane
	snapshot := panelLedgerFixture()
	snapshot.State = "frozen"
	snapshot.PendingAmendments = []ledgerapi.CodingPlanningAmendment{{OperationID: "owner-amendment", Revision: snapshot.Revision, Text: "Add the requested later behavior."}}
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	view.mu.Lock()
	lines := strings.Join(view.ledgerLinesLocked(100), "\n")
	view.mu.Unlock()
	for _, text := range []string{"Approved executable revision", "PENDING CHANGES", "awaiting revision and owner sign-off", "From revision", "Add the requested later behavior.", "APPROVED ITEMS"} {
		if !strings.Contains(lines, text) {
			t.Fatal("missing separation", text, lines)
		}
	}
	if len(view.ledger.snapshot.Ledger.Tasks) != len(snapshot.Ledger.Tasks) {
		t.Fatal("pending input became an executable item")
	}
	view.mu.Lock()
	narrow := view.ledgerLinesLocked(22)
	view.mu.Unlock()
	for _, line := range narrow {
		if ansi.StringWidth(line) > 22 {
			t.Fatal("pending content exceeds narrow pane", line)
		}
	}
	if err := view.key('j', false); err != nil {
		t.Fatal(err)
	}
	if err := view.key('a', false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.ledger.feedback, "read-only") {
		t.Fatal("pending amendment unlocked frozen controls")
	}
}
