package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingServiceStatePurposeCandidate(
	runtime typedWorkerRuntime,
	models directCodingServiceStateLeafModels,
	subject string,
	authority assemblyline.ApplicationServiceStateInterfaceInput,
	scope assemblyline.ApplicationServiceStatePurposeScope,
	parentPurpose string,
	candidatePurpose string,
	acceptedPurposes []string,
	identities []assemblyline.ArtifactIdentity,
) (bool, error) {
	for _, acceptedPurpose := range acceptedPurposes {
		if candidatePurpose == acceptedPurpose {
			return false, nil
		}
	}
	necessityInput := assemblyline.ApplicationServiceStatePurposeNecessityInput{
		Scope: scope, Authority: authority, ParentPurpose: parentPurpose,
		CandidatePurpose: candidatePurpose,
	}
	necessityJob, err := assemblyline.NewApplicationServiceStatePurposeNecessityJob(
		necessityInput,
	)
	if err != nil {
		return false, err
	}
	necessity, err := runDirectCodingSemanticLeafCall(
		runtime, models.PurposeNecessity, subject+"_purpose_necessity",
		necessityJob, identities,
		func(raw string) (assemblyline.ApplicationServiceStatePurposeNecessityResult, error) {
			return assemblyline.DecodeApplicationServiceStatePurposeNecessityResult(
				necessityInput, raw,
			)
		},
		func(result assemblyline.ApplicationServiceStatePurposeNecessityResult) error {
			return result.ValidateFor(necessityInput)
		},
	)
	if err != nil {
		return false, err
	}
	if necessity.Relation == assemblyline.ApplicationServiceStatePurposeNotNecessary {
		return false, nil
	}
	for acceptedIndex, acceptedPurpose := range acceptedPurposes {
		relationInput := assemblyline.ApplicationServiceStatePurposeRelationInput{
			Scope: scope, CandidatePurpose: candidatePurpose,
			AcceptedPurpose: acceptedPurpose,
		}
		relationJob, err := assemblyline.NewApplicationServiceStatePurposeRelationJob(
			relationInput,
		)
		if err != nil {
			return false, err
		}
		relation, err := runDirectCodingSemanticLeafCall(
			runtime, models.PurposeRelation,
			fmt.Sprintf("%s_purpose_relation_%03d", subject, acceptedIndex+1),
			relationJob, identities,
			func(raw string) (assemblyline.ApplicationServiceStatePurposeRelationResult, error) {
				return assemblyline.DecodeApplicationServiceStatePurposeRelationResult(
					relationInput, raw,
				)
			},
			func(result assemblyline.ApplicationServiceStatePurposeRelationResult) error {
				return result.ValidateFor(relationInput)
			},
		)
		if err != nil {
			return false, err
		}
		if relation.Relation == assemblyline.ApplicationServiceStateSamePurpose {
			return false, nil
		}
	}
	return true, nil
}

func resolveDirectCodingServiceRecordFields(
	runtime typedWorkerRuntime,
	models directCodingServiceStateLeafModels,
	subject string,
	authority assemblyline.ApplicationServiceStateInterfaceInput,
	parentPurpose string,
	identities []assemblyline.ArtifactIdentity,
) ([]assemblyline.ApplicationServiceStateRecordField, error) {
	inventoryInput := assemblyline.ApplicationRecordFieldPurposeInventoryInput{
		Authority: authority, ParentPurpose: parentPurpose,
	}
	inventoryJob, err := assemblyline.NewApplicationRecordFieldPurposeInventoryJob(
		inventoryInput,
	)
	if err != nil {
		return nil, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime, models.RecordInventory, subject+"_record_field_purpose_inventory",
		inventoryJob, identities,
		func(raw string) (assemblyline.ApplicationServiceStatePurposeInventory, error) {
			return assemblyline.DecodeApplicationRecordFieldPurposeInventory(inventoryInput, raw)
		},
		func(result assemblyline.ApplicationServiceStatePurposeInventory) error {
			return result.ValidateForRecordFields(inventoryInput)
		},
	)
	if err != nil {
		return nil, err
	}
	fields := make([]assemblyline.ApplicationServiceStateRecordField, 0, len(inventory.Purposes))
	seenCandidates := make(map[string]struct{}, len(inventory.Purposes))
	for candidateIndex, purpose := range inventory.Purposes {
		if _, duplicate := seenCandidates[purpose]; duplicate {
			continue
		}
		seenCandidates[purpose] = struct{}{}
		accepted, acceptErr := resolveDirectCodingServiceStatePurposeCandidate(
			runtime, models,
			fmt.Sprintf("%s_record_candidate_%03d", subject, candidateIndex+1),
			authority, assemblyline.ApplicationServiceStateRecordPurposeScope,
			parentPurpose, purpose, applicationServiceStateRecordFieldPurposes(fields),
			identities,
		)
		if acceptErr != nil {
			return nil, acceptErr
		}
		if !accepted {
			continue
		}
		name, err := assemblyline.CodeOwnedApplicationServiceRecordFieldName(len(fields) + 1)
		if err != nil {
			return nil, err
		}
		kindInput := assemblyline.ApplicationRecordFieldKindInput{
			Authority: authority, ParentPurpose: parentPurpose,
			FocusedPurpose: purpose,
		}
		kindJob, err := assemblyline.NewApplicationRecordFieldKindJob(kindInput)
		if err != nil {
			return nil, err
		}
		kind, err := runDirectCodingSemanticLeafCall(
			runtime, models.RecordKind,
			fmt.Sprintf("%s_record_field_kind_%03d", subject, candidateIndex+1),
			kindJob, identities,
			func(raw string) (assemblyline.ApplicationServiceStateFieldKind, error) {
				return assemblyline.DecodeApplicationRecordFieldKindLeaf(kindInput, raw)
			},
			func(assemblyline.ApplicationServiceStateFieldKind) error { return nil },
		)
		if err != nil {
			return nil, err
		}
		fields = append(fields, assemblyline.ApplicationServiceStateRecordField{
			Name: name, Purpose: purpose, Kind: kind,
		})
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("application record purpose inventory exhausted without one behavior-authorized distinct scalar purpose")
	}
	return fields, nil
}

func applicationServiceStateFieldPurposes(
	fields []assemblyline.ApplicationServiceStateField,
) []string {
	purposes := make([]string, len(fields))
	for index, field := range fields {
		purposes[index] = field.Purpose
	}
	return purposes
}

func applicationServiceStateRecordFieldPurposes(
	fields []assemblyline.ApplicationServiceStateRecordField,
) []string {
	purposes := make([]string, len(fields))
	for index, field := range fields {
		purposes[index] = field.Purpose
	}
	return purposes
}
