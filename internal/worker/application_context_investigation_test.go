package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationContextInvestigationResolvesNamedNeedWithoutAcceptanceCall(t *testing.T) {
	t.Parallel()
	const request = "Exclude archived patients from the existing patient search."
	initial, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			calls++
			if modelName != "context-model" || job.Kind != assemblyline.WorkApplicationContextNeeds {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context call kind=%q model=%q", job.Kind, modelName)
			}
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: `{"schema":"omnidex.application-context-needs.v1","questions":["Which symbol owns archived-patient filtering?"]}`,
			}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(need assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			if need.ID != "context_evidence_need_001" ||
				need.Question != "Which symbol owns archived-patient filtering?" ||
				need.Kind != assemblyline.ApplicationEvidenceContextFact ||
				need.StopCondition != assemblyline.ApplicationEvidenceRelevantSelection {
				return nil, fmt.Errorf("unexpected code-owned evidence need: %+v", need)
			}
			const fact = "PatientQuery::applyFilters owns archived-patient filtering."
			return []assemblyline.ApplicationContextEvidence{{
				Value: fact, SourceID: "symbol:PatientQuery::applyFilters",
				SourceSHA256: assemblyline.ExactObjectiveContextSHA(fact),
			}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("context investigation calls=%d; code must not request a model acceptance after resolving evidence", calls)
	}
	if len(resolved.Facts) != 2 || resolved.Facts[1].NeedID != "context_evidence_need_001" ||
		resolved.Facts[1].Authority != assemblyline.ApplicationContextEvidenceAuthority {
		t.Fatalf("resolved context=%+v", resolved)
	}
}
