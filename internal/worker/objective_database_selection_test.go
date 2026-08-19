package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type maximumDatabaseSchemaSelectionStations struct {
	*scriptedObjectiveDatabaseStations
}

func (station *maximumDatabaseSchemaSelectionStations) SelectSchema(
	_ context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	ids := make([]string, input.MaxSelections)
	for index := range ids {
		ids[index] = input.Candidates[index].RelationID
	}
	return assemblyline.DatabaseSchemaSelectionDecision{
		Schema: assemblyline.DatabaseSchemaSelectionV1, EvidenceNeedID: input.EvidenceNeedID,
		RelationIDs: ids,
	}, objectiveStationReceipt{Calls: maxTypedWorkerAttempts}, nil
}

func TestDatabaseSchemaReductionHasOneHardSemanticCallBound(t *testing.T) {
	candidates := make([]assemblyline.DatabaseSchemaCandidate, 2048)
	for index := range candidates {
		candidates[index] = assemblyline.DatabaseSchemaCandidate{
			RelationID: fmt.Sprintf("rel_%04d", index),
			Descriptor: fmt.Sprintf("schema relation %04d", index),
		}
	}
	station := &maximumDatabaseSchemaSelectionStations{
		scriptedObjectiveDatabaseStations: &scriptedObjectiveDatabaseStations{t: t},
	}
	_, calls, err := reduceObjectiveDatabaseCandidates(
		context.Background(), "need-1", "Find the exact relevant records.",
		assemblyline.ObjectiveContext{}, candidates, station,
	)
	if err == nil || !strings.Contains(err.Error(), "96-call semantic reduction bound") {
		t.Fatalf("schema reduction error=%v", err)
	}
	if calls != maxDatabaseSchemaSelectionModelCalls {
		t.Fatalf("schema reduction calls=%d want %d", calls, maxDatabaseSchemaSelectionModelCalls)
	}
}
