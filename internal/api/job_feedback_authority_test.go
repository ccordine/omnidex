package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFeedbackAuthorityRejectsMismatchedJob(t *testing.T) {
	err := validateSameJobAuthority(41, model.Job{ID: 42})
	if err == nil || !strings.Contains(err.Error(), "expected job 41") {
		t.Fatalf("mismatched job error=%v", err)
	}
	if err := validateSameJobAuthority(41, model.Job{ID: 41}); err != nil {
		t.Fatalf("same job rejected: %v", err)
	}
}

func TestLifecycleControlReceiptIsBoundedToSameJobAndOperation(t *testing.T) {
	operationID, err := queue.NewLifecycleOperationID("receipt-test", "41")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newLifecycleControlReceipt(41, operationID, model.Job{ID: 41, Status: model.JobStatusRunning})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"job_id":41,"operation_id":"` + string(operationID) + `","status":"running"}`
	if string(raw) != want {
		t.Fatalf("receipt=%s want=%s", raw, want)
	}
	if _, err := newLifecycleControlReceipt(41, operationID, model.Job{ID: 42, Status: model.JobStatusRunning}); err == nil {
		t.Fatal("mismatched job unexpectedly produced a success receipt")
	}
}

func TestFeedbackHTTPHasNoSuccessorJobFallback(t *testing.T) {
	for _, path := range []string{"server_llm.go", "server_jobs.go", "scrum_channel_operation.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"superseded_job_id",
			"Job superseded by revised user authority",
			"replaced job",
			"controlledJob.ID != jobID",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden successor-job fallback %q", path, forbidden)
			}
		}
	}
}
