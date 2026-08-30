package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestServiceStatePurposeInventoriesUseCodeOwnedQueuesAndPreserveAcceptedLeaves(t *testing.T) {
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
		FieldInventory: "field-inventory-model", FieldKind: "field-kind-model",
		RecordInventory: "record-inventory-model", RecordKind: "record-kind-model",
		PurposeNecessity: "purpose-necessity-model", PurposeRelation: "purpose-relation-model",
	}
	wantModel := map[assemblyline.WorkKind]string{
		assemblyline.WorkApplicationStateFieldPurposeInventory:   models.FieldInventory,
		assemblyline.WorkApplicationStateFieldKind:               models.FieldKind,
		assemblyline.WorkApplicationRecordFieldPurposeInventory:  models.RecordInventory,
		assemblyline.WorkApplicationRecordFieldKind:              models.RecordKind,
		assemblyline.WorkApplicationServiceStatePurposeNecessity: models.PurposeNecessity,
		assemblyline.WorkApplicationServiceStatePurposeRelation:  models.PurposeRelation,
	}
	const (
		rootRecords        = "The accepted inventory records."
		rootOptional       = "A decorative inventory badge."
		rootSemanticCopy   = "The inventory records retained for retrieval."
		rootCursor         = "The most recent retrieval cursor."
		recordIdentifier   = "The inventory record identifier."
		recordOptional     = "A decorative record badge."
		recordSemanticCopy = "The identifier for each inventory record."
		recordAmount       = "The recorded inventory amount."
	)
	calls := make(map[assemblyline.WorkKind]int, len(wantModel))
	necessityCandidates := make(map[string]int)
	relationCandidates := make(map[string]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 9, CorrectionModel: "forbidden-correction",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if model != wantModel[job.Kind] {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"work kind %q used model %q, want %q", job.Kind, model, wantModel[job.Kind],
				)
			}
			calls[job.Kind]++
			raw, err := serviceStateQueueTestResponse(
				job, necessityCandidates, relationCandidates,
				rootRecords, rootOptional, rootSemanticCopy, rootCursor,
				recordIdentifier, recordOptional, recordSemanticCopy, recordAmount,
			)
			if err != nil {
				return assemblyline.PortableResult{}, err
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
	fields := resolved.Interfaces[0].Result.Fields
	if len(fields) != 2 || fields[0].Name != "state_001" ||
		fields[0].Purpose != rootRecords ||
		fields[0].Kind != assemblyline.ApplicationServiceStateRecordList ||
		fields[1].Name != "state_002" || fields[1].Purpose != rootCursor ||
		fields[1].Kind != assemblyline.ApplicationServiceStateString {
		t.Fatalf("code-composed root fields=%+v", fields)
	}
	recordFields := fields[0].RecordFields
	if len(recordFields) != 2 || recordFields[0].Name != "member_001" ||
		recordFields[0].Purpose != recordIdentifier ||
		recordFields[0].Kind != assemblyline.ApplicationServiceStateString ||
		recordFields[1].Name != "member_002" ||
		recordFields[1].Purpose != recordAmount ||
		recordFields[1].Kind != assemblyline.ApplicationServiceStateNumber {
		t.Fatalf("code-composed record fields=%+v", recordFields)
	}
	wantCalls := map[assemblyline.WorkKind]int{
		assemblyline.WorkApplicationStateFieldPurposeInventory:   1,
		assemblyline.WorkApplicationRecordFieldPurposeInventory:  1,
		assemblyline.WorkApplicationServiceStatePurposeNecessity: 8,
		assemblyline.WorkApplicationServiceStatePurposeRelation:  4,
		assemblyline.WorkApplicationStateFieldKind:               2,
		assemblyline.WorkApplicationRecordFieldKind:              2,
	}
	for kind, want := range wantCalls {
		if calls[kind] != want {
			t.Fatalf("work kind %q calls=%d want=%d", kind, calls[kind], want)
		}
	}
	for _, exactDuplicate := range []string{
		rootRecords, rootOptional, recordIdentifier, recordOptional,
	} {
		if necessityCandidates[exactDuplicate] != 1 {
			t.Fatalf("exact duplicate %q reopened necessity: %+v", exactDuplicate, necessityCandidates)
		}
	}
	for _, semanticDuplicate := range []string{rootSemanticCopy, recordSemanticCopy} {
		if relationCandidates[semanticDuplicate] != 1 {
			t.Fatalf("semantic duplicate %q relation calls=%d", semanticDuplicate, relationCandidates[semanticDuplicate])
		}
	}
}

func serviceStateQueueTestResponse(
	job assemblyline.PortableJob,
	necessityCandidates map[string]int,
	relationCandidates map[string]int,
	rootRecords, rootOptional, rootSemanticCopy, rootCursor string,
	recordIdentifier, recordOptional, recordSemanticCopy, recordAmount string,
) (string, error) {
	switch job.Kind {
	case assemblyline.WorkApplicationStateFieldPurposeInventory:
		return rootRecords + "\n" + rootOptional + "\n" + rootRecords + "\n" + rootOptional + "\n" +
			rootSemanticCopy + "\n" + rootCursor, nil
	case assemblyline.WorkApplicationRecordFieldPurposeInventory:
		return recordIdentifier + "\n" + recordOptional + "\n" + recordIdentifier + "\n" + recordOptional + "\n" +
			recordSemanticCopy + "\n" + recordAmount, nil
	case assemblyline.WorkApplicationServiceStatePurposeNecessity:
		var input assemblyline.ApplicationServiceStatePurposeNecessityInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", err
		}
		necessityCandidates[input.CandidatePurpose]++
		switch input.CandidatePurpose {
		case rootOptional, recordOptional:
			return assemblyline.ApplicationServiceStatePurposeNotNecessary, nil
		default:
			return assemblyline.ApplicationServiceStatePurposeNecessary, nil
		}
	case assemblyline.WorkApplicationServiceStatePurposeRelation:
		var input assemblyline.ApplicationServiceStatePurposeRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", err
		}
		relationCandidates[input.CandidatePurpose]++
		switch input.CandidatePurpose {
		case rootSemanticCopy, recordSemanticCopy:
			return assemblyline.ApplicationServiceStateSamePurpose, nil
		default:
			return assemblyline.ApplicationServiceStateDistinctPurposes, nil
		}
	case assemblyline.WorkApplicationStateFieldKind:
		var input assemblyline.ApplicationStateFieldKindInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", err
		}
		if input.FocusedPurpose == rootRecords {
			return string(assemblyline.ApplicationServiceStateRecordList), nil
		}
		return string(assemblyline.ApplicationServiceStateString), nil
	case assemblyline.WorkApplicationRecordFieldKind:
		var input assemblyline.ApplicationRecordFieldKindInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", err
		}
		if input.FocusedPurpose == recordAmount {
			return string(assemblyline.ApplicationServiceStateNumber), nil
		}
		return string(assemblyline.ApplicationServiceStateString), nil
	default:
		return "", fmt.Errorf("unexpected work kind %q", job.Kind)
	}
}

func TestServiceStateScalarInventoryExhaustionDoesNotOpenRecordInventory(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceStateInterfaceInput{
		ProductContext: "reading preference service",
		Needs: []assemblyline.ApplicationServiceStateInterfaceNeed{{
			RequirementQuote: "Remember whether the reader prefers compact text across requests.",
		}},
	}
	models := directCodingServiceStateLeafModels{
		FieldInventory: "inventory", FieldKind: "kind", RecordInventory: "forbidden-record",
		RecordKind: "forbidden-record-kind", PurposeNecessity: "necessity", PurposeRelation: "relation",
	}
	calls := make(map[assemblyline.WorkKind]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls[job.Kind]++
			var raw string
			switch job.Kind {
			case assemblyline.WorkApplicationStateFieldPurposeInventory:
				raw = "Whether compact text is preferred."
			case assemblyline.WorkApplicationServiceStatePurposeNecessity:
				raw = assemblyline.ApplicationServiceStatePurposeNecessary
			case assemblyline.WorkApplicationStateFieldKind:
				raw = string(assemblyline.ApplicationServiceStateBoolean)
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected scalar work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
	}
	result, err := resolveDirectCodingServiceStateInterface(
		runtime, models, "preference", authority, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fields) != 1 || result.Fields[0].Kind != assemblyline.ApplicationServiceStateBoolean {
		t.Fatalf("result=%+v", result)
	}
	if calls[assemblyline.WorkApplicationRecordFieldPurposeInventory] != 0 ||
		calls[assemblyline.WorkApplicationServiceStatePurposeRelation] != 0 {
		t.Fatalf("unnecessary calls=%+v", calls)
	}
}
