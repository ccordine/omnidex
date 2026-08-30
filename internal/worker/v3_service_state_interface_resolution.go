package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingServiceStateLeafModels struct {
	FieldInventory   string
	FieldKind        string
	RecordInventory  string
	RecordKind       string
	PurposeNecessity string
	PurposeRelation  string
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
		{&models.FieldInventory, station.CodingApplicationStateFieldPurposeInventory},
		{&models.FieldKind, station.CodingApplicationStateFieldKind},
		{&models.RecordInventory, station.CodingApplicationRecordFieldPurposeInventory},
		{&models.RecordKind, station.CodingApplicationRecordFieldKind},
		{&models.PurposeNecessity, station.CodingApplicationServiceStatePurposeNecessity},
		{&models.PurposeRelation, station.CodingApplicationServiceStatePurposeRelation},
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
	inventoryInput := assemblyline.ApplicationStateFieldPurposeInventoryInput{
		Authority: authority,
	}
	inventoryJob, err := assemblyline.NewApplicationStateFieldPurposeInventoryJob(inventoryInput)
	if err != nil {
		return zero, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime, models.FieldInventory, subject+"_field_purpose_inventory",
		inventoryJob, identities,
		func(raw string) (assemblyline.ApplicationServiceStatePurposeInventory, error) {
			return assemblyline.DecodeApplicationStateFieldPurposeInventory(inventoryInput, raw)
		},
		func(result assemblyline.ApplicationServiceStatePurposeInventory) error {
			return result.ValidateForStateFields(inventoryInput)
		},
	)
	if err != nil {
		return zero, err
	}
	fields := make([]assemblyline.ApplicationServiceStateField, 0, len(inventory.Purposes))
	seenCandidates := make(map[string]struct{}, len(inventory.Purposes))
	for candidateIndex, purpose := range inventory.Purposes {
		if _, duplicate := seenCandidates[purpose]; duplicate {
			continue
		}
		seenCandidates[purpose] = struct{}{}
		accepted, acceptErr := resolveDirectCodingServiceStatePurposeCandidate(
			runtime, models, fmt.Sprintf("%s_field_candidate_%03d", subject, candidateIndex+1),
			authority, assemblyline.ApplicationServiceStateRootPurposeScope, "",
			purpose, applicationServiceStateFieldPurposes(fields), identities,
		)
		if acceptErr != nil {
			return zero, acceptErr
		}
		if !accepted {
			continue
		}
		name, err := assemblyline.CodeOwnedApplicationServiceStateFieldName(len(fields) + 1)
		if err != nil {
			return zero, err
		}
		kindInput := assemblyline.ApplicationStateFieldKindInput{
			Authority: authority, FocusedPurpose: purpose,
		}
		kindJob, err := assemblyline.NewApplicationStateFieldKindJob(kindInput)
		if err != nil {
			return zero, err
		}
		kind, err := runDirectCodingSemanticLeafCall(
			runtime, models.FieldKind,
			fmt.Sprintf("%s_field_kind_%03d", subject, candidateIndex+1),
			kindJob, identities,
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
	if len(fields) == 0 {
		return zero, fmt.Errorf("application service state purpose inventory exhausted without one behavior-authorized distinct root purpose")
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
