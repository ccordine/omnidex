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
		request, assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			calls++
			if modelName != "context-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context call kind=%q model=%q", job.Kind, modelName)
			}
			candidate := ""
			switch calls {
			case 1:
				if job.Kind != assemblyline.WorkApplicationContextNeedCoverage {
					return assemblyline.PortableResult{}, fmt.Errorf("first context kind=%q", job.Kind)
				}
				candidate = assemblyline.ApplicationContextNeedRemains
			case 2:
				if job.Kind != assemblyline.WorkApplicationContextNeedQuestion {
					return assemblyline.PortableResult{}, fmt.Errorf("second context kind=%q", job.Kind)
				}
				candidate = "Which symbol owns archived-patient filtering?"
			case 3:
				if job.Kind != assemblyline.WorkApplicationContextNeedCoverage {
					return assemblyline.PortableResult{}, fmt.Errorf("third context kind=%q", job.Kind)
				}
				candidate = assemblyline.ApplicationNoUncoveredContextNeed
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected extra context call")
			}
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: candidate,
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
	if calls != 3 {
		t.Fatalf("context investigation calls=%d; code must not request a model acceptance after resolving evidence", calls)
	}
	if len(resolved.Facts) != 2 || resolved.Facts[1].NeedID != "context_evidence_need_001" ||
		resolved.Facts[1].Authority != assemblyline.ApplicationContextEvidenceAuthority {
		t.Fatalf("resolved context=%+v", resolved)
	}
}
