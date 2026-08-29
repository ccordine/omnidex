package worker

import (
	"strings"
	"testing"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestExistingRepositoryEvidenceRequestOwnsItsRetrievalOperation(t *testing.T) {
	t.Parallel()
	request, err := newExistingRepositoryEvidenceRequest(
		17, "analysis_"+strings.Repeat("1", 64), "Value",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Operation != repositoryretrieval.OperationSemanticExcerpts ||
		request.Query != "Value" || request.ProjectID != 17 {
		t.Fatalf("code-owned request=%#v", request)
	}
	if _, err := newExistingRepositoryEvidenceRequest(17, request.AnalysisID, " Value "); err == nil {
		t.Fatal("untrimmed code-owned query reached repository machinery")
	}
}

func TestRecordExistingRepositoryEvidenceRejectsDifferentOpaqueQueryBinding(t *testing.T) {
	t.Parallel()
	pack := repositoryAcquisitionTestPack(t, "Owner")
	if err := (&directCodingSession{}).recordExistingRepositoryEvidence("DifferentOwner", pack); err == nil ||
		!strings.Contains(err.Error(), "typed retrieval request") {
		t.Fatalf("opaque query binding mismatch error=%v", err)
	}
	if err := (&directCodingSession{}).recordExistingRepositoryEvidence("Owner", pack); err == nil ||
		!strings.Contains(err.Error(), "requires a runtime") {
		t.Fatalf("missing record authority error=%v", err)
	}
}
