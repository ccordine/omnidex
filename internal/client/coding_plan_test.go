package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanTransportUsesExactPersistedReviewAuthority(t *testing.T) {
	t.Parallel()

	const jobID int64 = 73
	const generation int64 = 4
	const revision int64 = 8
	workspaceRoot := "/tmp/coding plan"
	channel := testCLIChannel(workspaceRoot)
	planQuery := url.Values{
		"workspace_root": {workspaceRoot}, "workspace_identity": {testWorkspaceIdentity},
	}.Encode()
	decisionOperationID := testOperationID(t, "coding-plan-decision")
	freezeOperationID := testOperationID(t, "coding-plan-freeze")
	review := codingPlanFixture(t, jobID, generation, revision, model.CodingPlanStateReview)
	decided := review
	decided.Revision++
	decided.UpdatedAt = decided.UpdatedAt.Add(time.Second)
	decided.Leaves = append([]model.CodingPlanLeaf(nil), review.Leaves...)
	decided.Leaves[0].Decision = model.CodingPlanDecisionApproved
	decided.Leaves[1].Decision = model.CodingPlanDecisionRejected
	frozen := decided
	frozen.Revision++
	frozen.State = model.CodingPlanStateFrozen
	frozen.UpdatedAt = frozen.UpdatedAt.Add(time.Second)
	frozenAt := frozen.UpdatedAt
	frozen.FrozenAt = &frozenAt
	reviewLedger := codingLedgerFixture(review)
	reviewLedger.LatestDelta = ledgerapi.CodingLedgerDelta{LedgerID: 1, FromRevision: 7, ToRevision: 8}
	decidedLedger := codingLedgerFixture(decided)
	decidedLedger.BaseRevision = revision
	frozenLedger := codingLedgerFixture(frozen)
	frozenLedger.State = "frozen"

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/jobs/73/plan":
			requireRequestAuthority(t, request, http.MethodGet, request.URL.Path, planQuery)
			query := request.URL.Query()
			if len(query) != 2 || len(query["workspace_root"]) != 1 ||
				query.Get("workspace_root") != workspaceRoot ||
				len(query["workspace_identity"]) != 1 ||
				query.Get("workspace_identity") != testWorkspaceIdentity {
				t.Errorf("coding plan read query = %#v", query)
			}
			writeJSON(t, writer, http.StatusOK, reviewLedger)
		case "/v1/jobs/73/plan/decisions":
			requireRequestAuthority(t, request, http.MethodPost, request.URL.Path, "")
			requireJSONBody(t, request, map[string]any{
				"operation_id":       string(decisionOperationID),
				"owner_turn_id":      "terminal-" + string(decisionOperationID),
				"owner_turn_text":    "Set the displayed item review state.",
				"revision":           float64(revision),
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
				"action": map[string]any{
					"kind": "set_review_state", "target_id": string(review.Leaves[0].ID),
					"review_state": "agreed", "rationale": "explicit terminal item control",
				},
			})
			writeJSON(t, writer, http.StatusOK, decidedLedger)
		case "/v1/jobs/73/plan/freeze":
			requireRequestAuthority(t, request, http.MethodPost, request.URL.Path, "")
			requireJSONBody(t, request, map[string]any{
				"operation_id":       string(freezeOperationID),
				"owner_turn_id":      "terminal-" + string(freezeOperationID),
				"owner_turn_text":    "Sign off the exact displayed planning revision.",
				"revision":           float64(decided.Revision),
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
			})
			writeJSON(t, writer, http.StatusOK, frozenLedger)
		default:
			t.Errorf("unexpected coding plan request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	apiClient := testClient(t, server.URL)
	actual, err := apiClient.CodingPlan(
		context.Background(), channel, testWorkspaceIdentity, jobID,
	)
	if err != nil || actual.Revision != revision {
		t.Fatalf("load coding plan = %#v, error %v", actual, err)
	}
	turn := PlanningTurn{OperationID: string(decisionOperationID), OwnerTurnID: "terminal-" + string(decisionOperationID),
		Text: "Set the displayed item review state.", ObservedRevision: revision}
	changed, err := apiClient.MutateCodingLedgerDirect(context.Background(), channel, testWorkspaceIdentity, jobID, turn,
		DirectPlanningAction{Kind: model.LedgerMutationReview, TargetID: model.LedgerItemID(review.Leaves[0].ID),
			ReviewState: model.LedgerReviewAgreed, Rationale: "explicit terminal item control"})
	if err != nil || changed.Revision != decided.Revision {
		t.Fatalf("persist coding ledger decision = %#v, error %v", changed, err)
	}
	freezeTurn := PlanningTurn{OperationID: string(freezeOperationID), OwnerTurnID: "terminal-" + string(freezeOperationID),
		Text: "Sign off the exact displayed planning revision.", ObservedRevision: changed.Revision}
	receipt, err := apiClient.FreezeCodingLedger(context.Background(), channel, testWorkspaceIdentity, jobID, freezeTurn)
	if err != nil || receipt.State != "frozen" {
		t.Fatalf("freeze coding ledger = %#v, error %v", receipt, err)
	}
}

func TestCodingPlanClientRejectsContradictoryServerAuthority(t *testing.T) {
	t.Parallel()

	channel := testCLIChannel("/tmp/coding-plan-invalid-response")
	valid := codingLedgerFixture(codingPlanFixture(t, 91, 1, 1, model.CodingPlanStateReview))
	tests := []struct {
		name   string
		mutate func(*ledgerapi.CodingLedgerRevision)
		want   string
	}{
		{name: "wrong job", mutate: func(plan *ledgerapi.CodingLedgerRevision) { plan.JobID++ }, want: "differs from requested authority"},
		{name: "duplicate leaf", mutate: func(plan *ledgerapi.CodingLedgerRevision) {
			plan.Ledger.Tasks[1].ID = plan.Ledger.Tasks[0].ID
		}, want: "duplicate task"},
		{name: "duplicate statement", mutate: func(plan *ledgerapi.CodingLedgerRevision) {
			plan.Ledger.Tasks[1].Goal = plan.Ledger.Tasks[0].Goal
		}, want: "repeats the statement"},
		{name: "unknown decision", mutate: func(plan *ledgerapi.CodingLedgerRevision) { plan.Ledger.Tasks[0].ReviewState = "maybe" }, want: "requires a review state"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := valid
			response.Ledger.Tasks = append([]model.LedgerTask(nil), valid.Ledger.Tasks...)
			test.mutate(&response)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writeJSON(t, writer, http.StatusOK, response)
			}))
			defer server.Close()
			_, err := testClient(t, server.URL).CodingPlan(
				context.Background(), channel, testWorkspaceIdentity, 91,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("coding plan response error = %v, want %q", err, test.want)
			}
		})
	}
}

func codingLedgerFixture(plan model.CodingPlan) ledgerapi.CodingLedgerRevision {
	ledger := model.EngineeringLedger{ID: "ledger-test", Revision: plan.Revision}
	for index, leaf := range plan.Leaves {
		reviewState := model.LedgerReviewProposed
		switch leaf.Decision {
		case model.CodingPlanDecisionApproved:
			reviewState = model.LedgerReviewAgreed
		case model.CodingPlanDecisionRejected:
			reviewState = model.LedgerReviewNeedsDiscussion
		}
		ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
			LedgerItem: model.LedgerItem{
				ID:         model.LedgerItemID(leaf.ID),
				Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "test", Revision: 1},
				Rationale:  "test", Rank: int64(index),
				Disposition: model.LedgerDispositionActive, ReviewState: reviewState,
			},
			Goal: leaf.Statement,
		})
	}
	state := "proposed"
	if plan.State == model.CodingPlanStateFrozen {
		state = "frozen"
	}
	return ledgerapi.CodingLedgerRevision{LedgerID: 1, JobID: plan.JobID, Generation: plan.Generation,
		Revision: plan.Revision, State: state, CreatedAt: plan.CreatedAt, UpdatedAt: plan.UpdatedAt, Ledger: ledger}
}

func TestCodingPlanProjectionPreservesOutOfScopeOwnerDisposition(t *testing.T) {
	plan := codingPlanFixture(t, 91, 1, 2, model.CodingPlanStateReview)
	revision := codingLedgerFixture(plan)
	revision.OwnerTurnDispositions = []model.OwnerStatementDisposition{{
		Kind: model.OwnerTurnOutOfScope, Reason: "deployment belongs to a separate objective",
	}}

	projected, err := projectCodingLedgerForTerminal(revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.OwnerTurnDispositions) != 1 ||
		projected.OwnerTurnDispositions[0].Kind != model.OwnerTurnOutOfScope ||
		projected.OwnerTurnDispositions[0].Reason != "deployment belongs to a separate objective" {
		t.Fatalf("terminal projection dispositions = %#v", projected.OwnerTurnDispositions)
	}
}

func TestCodingPlanMutationRejectsInvalidAuthorityBeforeTransport(t *testing.T) {
	t.Parallel()

	channel := testCLIChannel("/tmp/coding-plan-invalid")
	apiClient := testClient(t, "http://127.0.0.1:1")
	validOperationID := testOperationID(t, "valid-plan-authority")
	validLeafID, err := model.NewCodingPlanLeafID()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		operation  model.LifecycleOperationID
		generation int64
		revision   int64
		changes    []CodingPlanDecisionChange
	}{
		{name: "operation", operation: "invalid", generation: 1, revision: 1, changes: []CodingPlanDecisionChange{{LeafID: validLeafID, Decision: model.CodingPlanDecisionApproved}}},
		{name: "generation", operation: validOperationID, revision: 1, changes: []CodingPlanDecisionChange{{LeafID: validLeafID, Decision: model.CodingPlanDecisionApproved}}},
		{name: "revision", operation: validOperationID, generation: 1, changes: []CodingPlanDecisionChange{{LeafID: validLeafID, Decision: model.CodingPlanDecisionApproved}}},
		{name: "pending decision", operation: validOperationID, generation: 1, revision: 1, changes: []CodingPlanDecisionChange{{LeafID: validLeafID, Decision: model.CodingPlanDecisionPending}}},
		{name: "duplicate decision", operation: validOperationID, generation: 1, revision: 1, changes: []CodingPlanDecisionChange{{LeafID: validLeafID, Decision: model.CodingPlanDecisionApproved}, {LeafID: validLeafID, Decision: model.CodingPlanDecisionRejected}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := apiClient.DecideCodingPlan(
				context.Background(), channel, testWorkspaceIdentity, 73,
				test.operation, test.generation, test.revision, test.changes,
			)
			if err == nil {
				t.Fatal("invalid coding plan mutation unexpectedly reached transport")
			}
		})
	}
}

func codingPlanFixture(
	t *testing.T,
	jobID int64,
	generation int64,
	revision int64,
	state model.CodingPlanState,
) model.CodingPlan {
	t.Helper()
	now := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	leaves := make([]model.CodingPlanLeaf, 0, 3)
	values := []struct {
		statement string
		decision  model.CodingPlanDecision
	}{
		{"The software lets a user confirm the item.", model.CodingPlanDecisionPending},
		{"The confirmed state persists.", model.CodingPlanDecisionPending},
		{"Export the confirmed item to cloud storage.", model.CodingPlanDecisionRejected},
	}
	for _, value := range values {
		id, err := model.NewCodingPlanLeafID()
		if err != nil {
			t.Fatal(err)
		}
		leaves = append(leaves, model.CodingPlanLeaf{
			ID: id, Statement: value.statement,
			Decision: value.decision,
		})
	}
	return model.CodingPlan{
		JobID: jobID, Generation: generation, Revision: revision, State: state,
		Leaves: leaves, CreatedAt: now, UpdatedAt: now,
	}
}
