package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
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
			writeJSON(t, writer, http.StatusOK, review)
		case "/v1/jobs/73/plan/decisions":
			requireRequestAuthority(t, request, http.MethodPost, request.URL.Path, "")
			requireJSONBody(t, request, map[string]any{
				"operation_id":       string(decisionOperationID),
				"generation":         float64(generation),
				"revision":           float64(revision),
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
				"decisions": []any{map[string]any{
					"leaf_id": string(review.Leaves[0].ID), "decision": "approved",
				}},
			})
			writeJSON(t, writer, http.StatusOK, decided)
		case "/v1/jobs/73/plan/freeze":
			requireRequestAuthority(t, request, http.MethodPost, request.URL.Path, "")
			requireJSONBody(t, request, map[string]any{
				"operation_id":       string(freezeOperationID),
				"generation":         float64(generation),
				"revision":           float64(decided.Revision),
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
			})
			writeJSON(t, writer, http.StatusOK, CodingPlanFreezeReceipt{
				Plan: frozen, JobStatus: model.JobStatusRunning,
			})
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
	actual, err = apiClient.DecideCodingPlan(
		context.Background(), channel, testWorkspaceIdentity, jobID,
		decisionOperationID, generation, revision,
		[]CodingPlanDecisionChange{{
			LeafID: review.Leaves[0].ID, Decision: model.CodingPlanDecisionApproved,
		}},
	)
	if err != nil || actual.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("persist coding plan decision = %#v, error %v", actual, err)
	}
	receipt, err := apiClient.FreezeCodingPlan(
		context.Background(), channel, testWorkspaceIdentity, jobID,
		freezeOperationID, generation, actual.Revision,
	)
	if err != nil || receipt.Plan.State != model.CodingPlanStateFrozen ||
		receipt.JobStatus != model.JobStatusRunning {
		t.Fatalf("freeze coding plan = %#v, error %v", receipt, err)
	}
}

func TestCodingPlanClientRejectsContradictoryServerAuthority(t *testing.T) {
	t.Parallel()

	channel := testCLIChannel("/tmp/coding-plan-invalid-response")
	valid := codingPlanFixture(t, 91, 1, 1, model.CodingPlanStateReview)
	tests := []struct {
		name   string
		mutate func(*model.CodingPlan)
		want   string
	}{
		{name: "wrong job", mutate: func(plan *model.CodingPlan) { plan.JobID++ }, want: "differs from requested job"},
		{name: "unknown scope", mutate: func(plan *model.CodingPlan) { plan.ScopeMode = "arbitrary" }, want: "unsupported"},
		{name: "duplicate leaf", mutate: func(plan *model.CodingPlan) { plan.Leaves[1].ID = plan.Leaves[0].ID }, want: "does not match its exact statement"},
		{name: "unknown annotation", mutate: func(plan *model.CodingPlan) { plan.Leaves[0].Annotation = "invented" }, want: "unsupported"},
		{name: "unknown decision", mutate: func(plan *model.CodingPlan) { plan.Leaves[0].Decision = "maybe" }, want: "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := valid
			response.Leaves = append([]model.CodingPlanLeaf(nil), valid.Leaves...)
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

func TestCodingPlanMutationRejectsInvalidAuthorityBeforeTransport(t *testing.T) {
	t.Parallel()

	channel := testCLIChannel("/tmp/coding-plan-invalid")
	apiClient := testClient(t, "http://127.0.0.1:1")
	validOperationID := testOperationID(t, "valid-plan-authority")
	validLeafID, err := model.NewCodingPlanLeafID("The software lets a user confirm the item.")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		operation  queue.LifecycleOperationID
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
		statement  string
		annotation model.CodingPlanAnnotation
		decision   model.CodingPlanDecision
	}{
		{"The software lets a user confirm the item.", model.CodingPlanAnnotationGrounded, model.CodingPlanDecisionPending},
		{"The confirmed state persists.", model.CodingPlanAnnotationReasonableDerivation, model.CodingPlanDecisionPending},
		{"Export the confirmed item to cloud storage.", model.CodingPlanAnnotationConcreteConflict, model.CodingPlanDecisionRejected},
	}
	for _, value := range values {
		id, err := model.NewCodingPlanLeafID(value.statement)
		if err != nil {
			t.Fatal(err)
		}
		leaves = append(leaves, model.CodingPlanLeaf{
			ID: id, Statement: value.statement, Annotation: value.annotation,
			Decision: value.decision,
		})
	}
	return model.CodingPlan{
		JobID: jobID, Generation: generation, Revision: revision, State: state,
		ScopeMode: model.CodingScopeModeNormal, RequestSHA256: strings.Repeat("a", 64),
		Leaves: leaves, CreatedAt: now, UpdatedAt: now,
	}
}
