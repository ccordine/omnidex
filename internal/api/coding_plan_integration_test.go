package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

func TestCodingPlanAPICommitsServerAuthoritativeDecisionsAndFreeze(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for coding-plan API coverage")
	}
	repository := freshCLIChatAPIRepository(t, databaseURL)
	workspaceRoot := t.TempDir()
	workspaceIdentity, err := projectroot.DirectoryIdentity(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	const statement = "The finished software lets a user confirm the item."
	job, err := repository.EnqueueCodingJob(context.Background(), statement, workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(context.Background(), "coding-plan-api-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Step.Action != "v3_coding_plan" {
		t.Fatalf("coding plan claim=%#v", claim)
	}
	leafID, err := model.NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatal(err)
	}
	relation := apiCodingPlanResultRelation(t, statement)
	plan, err := repository.StoreCodingPlanReview(context.Background(), queue.StoreCodingPlanReviewCommand{
		Authority:     claim.Authority,
		ScopeMode:     model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA(statement),
		Leaves: []queue.CodingPlanLeafWrite{{
			Leaf: model.CodingPlanLeaf{
				ID: leafID, Statement: statement,
				Annotation: model.CodingPlanAnnotationGrounded,
				Decision:   model.CodingPlanDecisionPending,
			},
			DecisionOriginGeneration: 1,
			ResultRelation: &queue.CodingPlanResultRelationReceipt{
				Schema: relation.Schema, CandidateSHA256: relation.CandidateSHA256,
				KindReceiptSHA256:        relation.KindReceiptSHA256,
				CardinalityReceiptSHA256: relation.CardinalityReceiptSHA256,
				Relation:                 relation.Relation,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(repository, nil, ServerOptions{
		LifecycleContext: context.Background(), HostDirectoryAccessRoot: "/tmp",
		RealtimeStreamMaxAge: "1m", RealtimeHeartbeat: "1s", RealtimeWriteTimeout: "1s",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspaceRoot := t.TempDir()
	otherWorkspaceIdentity, err := projectroot.DirectoryIdentity(otherWorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus := func(method, path string, body []byte, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("%s %s status=%d body=%s, want %d", method, path, recorder.Code, recorder.Body.String(), want)
		}
	}
	for _, control := range []struct {
		action string
		field  string
		value  string
	}{
		{action: "feedback", field: "feedback", value: "Clarify this exact job."},
		{action: "interrupt", field: "feedback", value: "Pause this exact job."},
		{action: "replan", field: "feedback", value: "Revise this exact job."},
		{action: "cancel", field: "reason", value: "Cancel this exact job."},
	} {
		control := control
		operationID, err := queue.NewLifecycleOperationID(
			"coding-plan-api", "missing-"+control.action+"-workspace", jsonInt(job.ID),
		)
		if err != nil {
			t.Fatal(err)
		}
		missingBody, err := json.Marshal(map[string]any{
			"operation_id": operationID,
			control.field:  control.value,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(
			http.MethodPost,
			"/v1/jobs/"+jsonInt(job.ID)+"/"+control.action,
			missingBody,
			http.StatusBadRequest,
		)

		operationID, err = queue.NewLifecycleOperationID(
			"coding-plan-api", "wrong-"+control.action+"-workspace", jsonInt(job.ID),
		)
		if err != nil {
			t.Fatal(err)
		}
		wrongBody, err := json.Marshal(map[string]any{
			"operation_id":       operationID,
			control.field:        control.value,
			"workspace_root":     otherWorkspaceRoot,
			"workspace_identity": otherWorkspaceIdentity,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(
			http.MethodPost,
			"/v1/jobs/"+jsonInt(job.ID)+"/"+control.action,
			wrongBody,
			http.StatusConflict,
		)
	}
	planPath := "/v1/jobs/" + jsonInt(job.ID) + "/plan"
	requireStatus(http.MethodGet, planPath, nil, http.StatusBadRequest)
	wrongPlanQuery := url.Values{
		"workspace_root":     {otherWorkspaceRoot},
		"workspace_identity": {otherWorkspaceIdentity},
	}
	requireStatus(
		http.MethodGet, planPath+"?"+wrongPlanQuery.Encode(), nil, http.StatusConflict,
	)
	missingDecisionID, err := queue.NewLifecycleOperationID("coding-plan-api", "missing-authority", jsonInt(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	missingDecisionBody, err := json.Marshal(map[string]any{
		"operation_id": missingDecisionID,
		"generation":   plan.Generation,
		"revision":     plan.Revision,
		"decisions": []map[string]any{{
			"leaf_id": leafID, "decision": model.CodingPlanDecisionApproved,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(
		http.MethodPost, planPath+"/decisions", missingDecisionBody, http.StatusBadRequest,
	)
	wrongDecisionID, err := queue.NewLifecycleOperationID("coding-plan-api", "wrong-authority", jsonInt(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	wrongDecisionBody, err := json.Marshal(map[string]any{
		"operation_id":       wrongDecisionID,
		"generation":         plan.Generation,
		"revision":           plan.Revision,
		"workspace_root":     otherWorkspaceRoot,
		"workspace_identity": otherWorkspaceIdentity,
		"decisions": []map[string]any{{
			"leaf_id": leafID, "decision": model.CodingPlanDecisionApproved,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(
		http.MethodPost, planPath+"/decisions", wrongDecisionBody, http.StatusConflict,
	)

	read := httptest.NewRecorder()
	planQuery := url.Values{
		"workspace_root": {workspaceRoot}, "workspace_identity": {workspaceIdentity},
	}
	server.Handler().ServeHTTP(read, httptest.NewRequest(
		http.MethodGet, "/v1/jobs/"+jsonInt(job.ID)+"/plan?"+planQuery.Encode(), nil,
	))
	if read.Code != http.StatusOK {
		t.Fatalf("read coding plan status=%d body=%s", read.Code, read.Body.String())
	}
	var readPlan model.CodingPlan
	if err := json.Unmarshal(read.Body.Bytes(), &readPlan); err != nil {
		t.Fatal(err)
	}
	if readPlan.JobID != job.ID || readPlan.Revision != plan.Revision ||
		readPlan.Leaves[0].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("read plan=%+v", readPlan)
	}

	decisionID, err := queue.NewLifecycleOperationID("coding-plan-api", "decide", jsonInt(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	decideBody, err := json.Marshal(map[string]any{
		"operation_id":       decisionID,
		"generation":         plan.Generation,
		"revision":           plan.Revision,
		"workspace_root":     workspaceRoot,
		"workspace_identity": workspaceIdentity,
		"decisions": []map[string]any{{
			"leaf_id": leafID, "decision": model.CodingPlanDecisionApproved,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	decide := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/jobs/"+jsonInt(job.ID)+"/plan/decisions",
		bytes.NewReader(decideBody),
	)
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(decide, request)
	if decide.Code != http.StatusOK {
		t.Fatalf("decide coding plan status=%d body=%s", decide.Code, decide.Body.String())
	}
	var decided model.CodingPlan
	if err := json.Unmarshal(decide.Body.Bytes(), &decided); err != nil {
		t.Fatal(err)
	}
	if decided.Revision != plan.Revision+1 ||
		decided.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("decided plan=%+v", decided)
	}
	missingFreezeID, err := queue.NewLifecycleOperationID("coding-plan-api", "missing-freeze-authority", jsonInt(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	missingFreezeBody, err := json.Marshal(map[string]any{
		"operation_id": missingFreezeID,
		"generation":   decided.Generation,
		"revision":     decided.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(
		http.MethodPost, planPath+"/freeze", missingFreezeBody, http.StatusBadRequest,
	)
	wrongFreezeID, err := queue.NewLifecycleOperationID("coding-plan-api", "wrong-freeze-authority", jsonInt(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	wrongFreezeBody, err := json.Marshal(map[string]any{
		"operation_id":       wrongFreezeID,
		"generation":         decided.Generation,
		"revision":           decided.Revision,
		"workspace_root":     otherWorkspaceRoot,
		"workspace_identity": otherWorkspaceIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(
		http.MethodPost, planPath+"/freeze", wrongFreezeBody, http.StatusConflict,
	)

	freezeID, err := queue.NewLifecycleOperationID("coding-plan-api", "freeze", jsonInt(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	freezeBody, err := json.Marshal(map[string]any{
		"operation_id":       freezeID,
		"generation":         decided.Generation,
		"revision":           decided.Revision,
		"workspace_root":     workspaceRoot,
		"workspace_identity": workspaceIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	freeze := httptest.NewRecorder()
	request = httptest.NewRequest(
		http.MethodPost,
		"/v1/jobs/"+jsonInt(job.ID)+"/plan/freeze",
		bytes.NewReader(freezeBody),
	)
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(freeze, request)
	if freeze.Code != http.StatusOK {
		t.Fatalf("freeze coding plan status=%d body=%s", freeze.Code, freeze.Body.String())
	}
	var receipt struct {
		Plan      model.CodingPlan `json:"plan"`
		JobStatus string           `json:"job_status"`
	}
	if err := json.Unmarshal(freeze.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Plan.State != model.CodingPlanStateFrozen ||
		receipt.JobStatus != model.JobStatusRunning {
		t.Fatalf("freeze receipt=%+v", receipt)
	}
	next, err := repository.ClaimNextStep(context.Background(), "coding-plan-api-execution")
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.Job.ID != job.ID || next.Step.Action != "v3_coding" {
		t.Fatalf("coding claim after API freeze=%#v", next)
	}
}

func apiCodingPlanResultRelation(
	t testing.TB,
	candidate string,
) assemblyline.ApplicationRequirementCandidateResultRelationResult {
	t.Helper()
	runtimeInput := assemblyline.ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate, Dimension: assemblyline.ApplicationRequirementCandidateRuntimeContentDimension,
	}
	runtimeContent, err := assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult(runtimeInput, "A")
	if err != nil {
		t.Fatal(err)
	}
	nonRuntimeInput := assemblyline.ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate, Dimension: assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension,
	}
	nonRuntimeContent, err := assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult(nonRuntimeInput, "B")
	if err != nil {
		t.Fatal(err)
	}
	kind, resolved, err := assemblyline.ResolveApplicationRequirementCandidateKind(
		candidate, runtimeContent, nonRuntimeContent,
	)
	if err != nil || !resolved {
		t.Fatalf("resolve fixture candidate kind: resolved=%t error=%v", resolved, err)
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: candidate}, "A",
	)
	if err != nil {
		t.Fatal(err)
	}
	relationInput := assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
	derived, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
		assemblyline.ApplicationRequirementCandidateResultPresenceInput{
			Candidate: candidate, Kind: kind, Cardinality: cardinality,
			Dimension: assemblyline.ApplicationRequirementDerivedValueDimension,
		},
		"B",
	)
	if err != nil {
		t.Fatal(err)
	}
	relation, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(
		relationInput, derived, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return relation
}

func jsonInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
