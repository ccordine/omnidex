package worker

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryEvidenceRelevanceRestoresEveryExactRelationWithoutProviderCalls(t *testing.T) {
	t.Parallel()
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		if request.Job.Kind != assemblyline.WorkRepositoryEvidenceRelevanceRelation ||
			request.Station != station.RepositoryEvidenceRelevance {
			t.Fatalf("reuse request=%+v", request)
		}
		raw := assemblyline.RepositoryEvidenceDirectlyRelevant
		projection, err := assemblyline.NewExactPortableResultProjection(raw)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: raw, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	evidence := []objectiveEvidence{
		mustObjectiveEvidence(t, "R01", "DispatchClock owns dispatch timing.", "repository_symbol", "pack#clock"),
		mustObjectiveEvidence(t, "R02", "ScheduleDispatch reads DispatchClock.", "repository_relation", "pack#schedule-clock"),
	}

	decision, receipt, err := runtime.resolveObjectiveRepositoryRelevance(
		t.Context(), "Identify the declarations that establish dispatch timing.", evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 2 || receipt.Calls != 0 || !receipt.Reused ||
		!reflect.DeepEqual(decision.EvidenceIDs, []string{"R01", "R02"}) {
		t.Fatalf("reuse_calls=%d receipt=%+v decision=%+v", reuseCalls, receipt, decision)
	}
}
