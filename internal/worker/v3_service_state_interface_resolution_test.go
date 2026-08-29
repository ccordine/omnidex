package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestServiceStateInterfaceResolutionComposesNarrowSemanticLeavesWithCodeOwnedNames(t *testing.T) {
	t.Parallel()
	_, workload := serviceStateWorkloadFixture(t)
	capabilities := directCodingCapabilityGraph{
		workload.Tasks[0].RequirementID: nil,
		workload.Tasks[1].RequirementID: nil,
	}
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	models := directCodingServiceStateLeafModels{
		FieldCoverage:  "field-coverage-model",
		FieldPurpose:   "field-purpose-model",
		FieldKind:      "field-kind-model",
		RecordCoverage: "record-coverage-model",
		RecordPurpose:  "record-purpose-model",
		RecordKind:     "record-kind-model",
	}
	wantModel := map[assemblyline.WorkKind]string{
		assemblyline.WorkApplicationStateFieldCoverage:  models.FieldCoverage,
		assemblyline.WorkApplicationStateFieldPurpose:   models.FieldPurpose,
		assemblyline.WorkApplicationStateFieldKind:      models.FieldKind,
		assemblyline.WorkApplicationRecordFieldCoverage: models.RecordCoverage,
		assemblyline.WorkApplicationRecordFieldPurpose:  models.RecordPurpose,
		assemblyline.WorkApplicationRecordFieldKind:     models.RecordKind,
	}
	calls := make(map[assemblyline.WorkKind]int, len(wantModel))
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 9, CorrectionModel: "forbidden-correction",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if model != wantModel[job.Kind] {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"work kind %q used model %q, want %q", job.Kind, model, wantModel[job.Kind],
				)
			}
			calls[job.Kind]++
			var raw string
			switch job.Kind {
			case assemblyline.WorkApplicationStateFieldPurpose:
				raw = "The accepted inventory records."
			case assemblyline.WorkApplicationStateFieldKind:
				raw = string(assemblyline.ApplicationServiceStateRecordList)
			case assemblyline.WorkApplicationRecordFieldPurpose:
				raw = "The inventory record identifier."
			case assemblyline.WorkApplicationRecordFieldKind:
				raw = string(assemblyline.ApplicationServiceStateString)
			case assemblyline.WorkApplicationRecordFieldCoverage:
				raw = assemblyline.ApplicationNoUncoveredRecordField
			case assemblyline.WorkApplicationStateFieldCoverage:
				raw = assemblyline.ApplicationNoUncoveredStateField
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
	}

	resolved, err := resolveDirectCodingServiceStateInterfaces(
		runtime, models, workload, capabilities, plan, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Interfaces) != 1 {
		t.Fatalf("interfaces=%+v", resolved.Interfaces)
	}
	fields := resolved.Interfaces[0].Result.Fields
	if len(fields) != 1 || fields[0].Name != "state_001" ||
		fields[0].Purpose != "The accepted inventory records." ||
		fields[0].Kind != assemblyline.ApplicationServiceStateRecordList ||
		len(fields[0].RecordFields) != 1 ||
		fields[0].RecordFields[0].Name != "member_001" ||
		fields[0].RecordFields[0].Purpose != "The inventory record identifier." ||
		fields[0].RecordFields[0].Kind != assemblyline.ApplicationServiceStateString {
		t.Fatalf("code-composed state interface=%+v", fields)
	}
	for kind := range wantModel {
		if calls[kind] != 1 {
			t.Fatalf("work kind %q calls=%d want=1", kind, calls[kind])
		}
	}
}
