package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingServiceStateLeafModels struct {
	FieldCoverage  string
	FieldPurpose   string
	FieldKind      string
	RecordCoverage string
	RecordPurpose  string
	RecordKind     string
}

type directCodingServiceStateInterfaceBinding struct {
	ID      string
	TaskIDs []string
	Input   assemblyline.ApplicationServiceStateInterfaceInput
	Result  assemblyline.ApplicationServiceStateInterfaceResult
}

type directCodingServiceStateComponent struct {
	ID      string
	TaskIDs []string
	Input   assemblyline.ApplicationServiceStateInterfaceInput
}

func (s *directCodingSession) resolveServiceStateInterfaces(
	runtime typedWorkerRuntime,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceStatePlan, error) {
	components, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	if len(components) == 0 {
		plan.Interfaces = nil
		plan.InterfaceByTask = nil
		return plan, nil
	}
	models := directCodingServiceStateLeafModels{}
	for _, binding := range []struct {
		destination *string
		stationID   station.ID
	}{
		{&models.FieldCoverage, station.CodingApplicationStateFieldCoverage},
		{&models.FieldPurpose, station.CodingApplicationStateFieldPurpose},
		{&models.FieldKind, station.CodingApplicationStateFieldKind},
		{&models.RecordCoverage, station.CodingApplicationRecordFieldCoverage},
		{&models.RecordPurpose, station.CodingApplicationRecordFieldPurpose},
		{&models.RecordKind, station.CodingApplicationRecordFieldKind},
	} {
		model, modelErr := s.workerModel(binding.stationID)
		if modelErr != nil {
			return directCodingServiceStatePlan{}, modelErr
		}
		*binding.destination = model
	}
	resolved, err := resolveDirectCodingServiceStateInterfaces(
		runtime, models, workload, capabilities, plan, identities,
	)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	return resolved, nil
}

func resolveDirectCodingServiceStateInterfaces(
	runtime typedWorkerRuntime,
	models directCodingServiceStateLeafModels,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceStatePlan, error) {
	components, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	if len(components) == 0 {
		plan.Interfaces = nil
		plan.InterfaceByTask = nil
		return plan, nil
	}
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	plan.Interfaces = make([]directCodingServiceStateInterfaceBinding, 0, len(components))
	plan.InterfaceByTask = make(map[string]string)
	for _, component := range components {
		result, err := resolveDirectCodingServiceStateInterface(
			runtime, models, "application_service_"+component.ID,
			component.Input, identities,
		)
		if err != nil {
			return directCodingServiceStatePlan{}, fmt.Errorf(
				"resolve service state interface %s: %w", component.ID, err,
			)
		}
		plan.Interfaces = append(plan.Interfaces, directCodingServiceStateInterfaceBinding{
			ID: component.ID, TaskIDs: append([]string(nil), component.TaskIDs...),
			Input: component.Input, Result: result,
		})
		for _, taskID := range component.TaskIDs {
			plan.InterfaceByTask[taskID] = component.ID
		}
	}
	if err := plan.ValidateInterfacesFor(workload, capabilities); err != nil {
		return directCodingServiceStatePlan{}, err
	}
	return plan, nil
}

func resolveDirectCodingServiceStateInterface(
	runtime typedWorkerRuntime,
	models directCodingServiceStateLeafModels,
	subject string,
	authority assemblyline.ApplicationServiceStateInterfaceInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationServiceStateInterfaceResult, error) {
	var zero assemblyline.ApplicationServiceStateInterfaceResult
	fields := make(
		[]assemblyline.ApplicationServiceStateField, 0,
		assemblyline.MaxApplicationServiceStateInterfaceFields,
	)
	for {
		leafInput := assemblyline.ApplicationStateFieldLeafInput{
			Authority: authority,
			AcceptedFields: append(
				[]assemblyline.ApplicationServiceStateField{}, fields...,
			),
		}
		if len(fields) > 0 {
			coverageJob, err := assemblyline.NewApplicationStateFieldCoverageJob(leafInput)
			if err != nil {
				return zero, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, models.FieldCoverage, subject+"_field_coverage", coverageJob, identities,
				func(raw string) (string, error) {
					return assemblyline.DecodeApplicationStateFieldCoverageLeaf(leafInput, raw)
				},
				func(string) error { return nil },
			)
			if err != nil {
				return zero, err
			}
			if coverage == assemblyline.ApplicationNoUncoveredStateField {
				break
			}
		}
		if len(fields) == assemblyline.MaxApplicationServiceStateInterfaceFields {
			return zero, fmt.Errorf(
				"application service state field coverage remains incomplete at the code-owned %d-field bound",
				assemblyline.MaxApplicationServiceStateInterfaceFields,
			)
		}

		purposeJob, err := assemblyline.NewApplicationStateFieldPurposeJob(leafInput)
		if err != nil {
			return zero, err
		}
		purpose, err := runDirectCodingSemanticLeafCall(
			runtime, models.FieldPurpose, subject+"_field_purpose", purposeJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationStateFieldPurposeLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return zero, err
		}
		name, err := assemblyline.CodeOwnedApplicationServiceStateFieldName(len(fields) + 1)
		if err != nil {
			return zero, err
		}
		kindInput := assemblyline.ApplicationStateFieldKindInput{
			Authority: authority,
			AcceptedFields: append(
				[]assemblyline.ApplicationServiceStateField{}, fields...,
			),
			FocusedPurpose: purpose,
		}
		kindJob, err := assemblyline.NewApplicationStateFieldKindJob(kindInput)
		if err != nil {
			return zero, err
		}
		kind, err := runDirectCodingSemanticLeafCall(
			runtime, models.FieldKind, subject+"_field_kind", kindJob, identities,
			func(raw string) (assemblyline.ApplicationServiceStateFieldKind, error) {
				return assemblyline.DecodeApplicationStateFieldKindLeaf(kindInput, raw)
			},
			func(assemblyline.ApplicationServiceStateFieldKind) error { return nil },
		)
		if err != nil {
			return zero, err
		}
		field := assemblyline.ApplicationServiceStateField{
			Name: name, Purpose: purpose, Kind: kind,
			RecordFields: []assemblyline.ApplicationServiceStateRecordField{},
		}
		if kind == assemblyline.ApplicationServiceStateRecordList {
			recordFields, recordErr := resolveDirectCodingServiceRecordFields(
				runtime, models, subject+"_"+name, authority, purpose, identities,
			)
			if recordErr != nil {
				return zero, recordErr
			}
			field.RecordFields = recordFields
		}
		fields = append(fields, field)
	}
	result := assemblyline.ApplicationServiceStateInterfaceResult{
		Schema: assemblyline.ApplicationServiceStateInterfaceSchemaV2,
		Fields: fields,
	}
	if err := result.ValidateFor(authority); err != nil {
		return zero, err
	}
	return result, nil
}

func resolveDirectCodingServiceRecordFields(
	runtime typedWorkerRuntime,
	models directCodingServiceStateLeafModels,
	subject string,
	authority assemblyline.ApplicationServiceStateInterfaceInput,
	parentPurpose string,
	identities []assemblyline.ArtifactIdentity,
) ([]assemblyline.ApplicationServiceStateRecordField, error) {
	fields := make(
		[]assemblyline.ApplicationServiceStateRecordField, 0,
		assemblyline.MaxApplicationServiceStateInterfaceFields,
	)
	for {
		leafInput := assemblyline.ApplicationRecordFieldLeafInput{
			Authority: authority, ParentPurpose: parentPurpose,
			AcceptedRecordFields: append(
				[]assemblyline.ApplicationServiceStateRecordField{}, fields...,
			),
		}
		if len(fields) > 0 {
			coverageJob, err := assemblyline.NewApplicationRecordFieldCoverageJob(leafInput)
			if err != nil {
				return nil, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, models.RecordCoverage, subject+"_record_field_coverage", coverageJob, identities,
				func(raw string) (string, error) {
					return assemblyline.DecodeApplicationRecordFieldCoverageLeaf(leafInput, raw)
				},
				func(string) error { return nil },
			)
			if err != nil {
				return nil, err
			}
			if coverage == assemblyline.ApplicationNoUncoveredRecordField {
				return fields, nil
			}
		}
		if len(fields) == assemblyline.MaxApplicationServiceStateInterfaceFields {
			return nil, fmt.Errorf(
				"application record field coverage remains incomplete at the code-owned %d-field bound",
				assemblyline.MaxApplicationServiceStateInterfaceFields,
			)
		}

		purposeJob, err := assemblyline.NewApplicationRecordFieldPurposeJob(leafInput)
		if err != nil {
			return nil, err
		}
		purpose, err := runDirectCodingSemanticLeafCall(
			runtime, models.RecordPurpose, subject+"_record_field_purpose", purposeJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationRecordFieldPurposeLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return nil, err
		}
		name, err := assemblyline.CodeOwnedApplicationServiceRecordFieldName(len(fields) + 1)
		if err != nil {
			return nil, err
		}
		kindInput := assemblyline.ApplicationRecordFieldKindInput{
			Authority: authority, ParentPurpose: parentPurpose,
			AcceptedRecordFields: append(
				[]assemblyline.ApplicationServiceStateRecordField{}, fields...,
			),
			FocusedPurpose: purpose,
		}
		kindJob, err := assemblyline.NewApplicationRecordFieldKindJob(kindInput)
		if err != nil {
			return nil, err
		}
		kind, err := runDirectCodingSemanticLeafCall(
			runtime, models.RecordKind, subject+"_record_field_kind", kindJob, identities,
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
}

func directCodingServiceStateComponents(
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
) ([]directCodingServiceStateComponent, error) {
	if err := plan.ValidateFor(workload); err != nil {
		return nil, err
	}
	requirements := make([]assemblyline.Requirement, 0, len(workload.Tasks))
	for _, task := range workload.Tasks {
		requirements = append(requirements, assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		})
	}
	if err := validateDirectCodingCapabilityGraph(requirements, capabilities); err != nil {
		return nil, err
	}
	components := make([]directCodingServiceStateComponent, 0)
	interfaceByTask := make(map[string]string)
	for durableIndex, durableTask := range workload.Tasks {
		if plan.ByTask[durableTask.ID] !=
			assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
			continue
		}
		component := directCodingServiceStateComponent{
			ID: fmt.Sprintf("state_interface_%03d", len(components)+1),
			Input: assemblyline.ApplicationServiceStateInterfaceInput{
				ProductContext: workload.ProductQuote,
			},
		}
		for candidateIndex, task := range workload.Tasks {
			included := candidateIndex == durableIndex
			if !included {
				for _, dependency := range capabilities[task.RequirementID] {
					if dependency.RequirementID == durableTask.RequirementID {
						included = true
						break
					}
				}
			}
			if !included {
				continue
			}
			if existing, duplicate := interfaceByTask[task.ID]; duplicate {
				return nil, fmt.Errorf(
					"service state task %s requires overlapping direct durable interfaces %s and %s",
					task.ID, existing, component.ID,
				)
			}
			authority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
				workload, task.ID,
			)
			if err != nil {
				return nil, err
			}
			need, err := assemblyline.ProjectApplicationServiceStateInterfaceNeed(authority)
			if err != nil {
				return nil, err
			}
			component.TaskIDs = append(component.TaskIDs, task.ID)
			component.Input.Needs = append(component.Input.Needs, need)
			interfaceByTask[task.ID] = component.ID
		}
		if err := component.Input.Validate(); err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, nil
}
