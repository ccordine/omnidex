package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestObjectiveSoleChoiceSkipsModelResolutionAndExecution(t *testing.T) {
	t.Parallel()
	input := soleJoinPathInput()
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	modelResolutions := 0
	decision, receipt, err := runObjectivePortableRawLeafStation(
		context.Background(), nil, "sole_join_path", job, station.DatabaseJoinPathSelection,
		func() (string, error) {
			modelResolutions++
			return "should-not-resolve", nil
		},
		func(raw string) (assemblyline.DatabaseJoinPathSelectionDecision, error) {
			return assemblyline.DecodeDatabaseJoinPathSelectionDecision(input, raw)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelResolutions != 0 || receipt.Calls != 0 {
		t.Fatalf("model resolutions=%d provider calls=%d", modelResolutions, receipt.Calls)
	}
	if decision.PathID != "customer-orders" {
		t.Fatalf("selected path = %q", decision.PathID)
	}
}

func TestExactStationBoundaryConsumesSoleChoiceWithoutProviderAuthority(t *testing.T) {
	t.Parallel()
	input := soleJoinPathInput()
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	result, execution, err := (&Service{}).executeExactPortableStation(
		context.Background(), model.StepAttemptAuthority{}, job, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidate != "A" || execution.Candidate != "A" ||
		execution.ProviderCalls != 0 || execution.CallEvidenceID != 0 {
		t.Fatalf("result=%#v execution=%#v", result, execution)
	}
}

func TestProviderDispatchRequiresRegisteredSemanticUncertainty(t *testing.T) {
	t.Parallel()
	_, _, err := (&Service{}).dispatchExactStationCall(
		context.Background(), model.StepAttemptAuthority{},
		exactStationCall{WorkKind: assemblyline.WorkKind("unregistered")},
		llm.PreparedModel{},
	)
	if err == nil || !strings.Contains(err.Error(), "semantic uncertainty") {
		t.Fatalf("dispatch admission error = %v", err)
	}
}

func soleJoinPathInput() assemblyline.DatabaseJoinPathSelectionInput {
	return assemblyline.DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Read the orders belonging to each customer.",
		Context:        assemblyline.ObjectiveContext{},
		FromRelationID: "customers",
		ToRelationID:   "orders",
		Candidates: []assemblyline.DatabaseJoinPathCandidate{
			{
				PathID:     "customer-orders",
				Descriptor: "Customer records relate directly to order records.",
			},
		},
	}
}
