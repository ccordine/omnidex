package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

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
	model, err := s.workerModel(station.CodingServiceStateInterface)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	return resolveDirectCodingServiceStateInterfaces(
		runtime, model, workload, capabilities, plan, identities,
	)
}

func resolveDirectCodingServiceStateInterfaces(
	runtime typedWorkerRuntime,
	model string,
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
			runtime, model, "application_service_"+component.ID,
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
	model string,
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
				runtime, model, subject+"_field_coverage", coverageJob, identities,
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

		nameJob, err := assemblyline.NewApplicationStateFieldNameJob(leafInput)
		if err != nil {
			return zero, err
		}
		name, err := runDirectCodingSemanticLeafCall(
			runtime, model, subject+"_field_name", nameJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationStateFieldNameLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return zero, err
		}
		kindInput := assemblyline.ApplicationStateFieldKindInput{
			Authority: authority,
			AcceptedFields: append(
				[]assemblyline.ApplicationServiceStateField{}, fields...,
			),
			FocusedName: name,
		}
		kindJob, err := assemblyline.NewApplicationStateFieldKindJob(kindInput)
		if err != nil {
			return zero, err
		}
		kind, err := runDirectCodingSemanticLeafCall(
			runtime, model, subject+"_field_kind", kindJob, identities,
			func(raw string) (assemblyline.ApplicationServiceStateFieldKind, error) {
				return assemblyline.DecodeApplicationStateFieldKindLeaf(kindInput, raw)
			},
			func(assemblyline.ApplicationServiceStateFieldKind) error { return nil },
		)
		if err != nil {
			return zero, err
		}
		field := assemblyline.ApplicationServiceStateField{
			Name: name, Kind: kind,
			RecordFields: []assemblyline.ApplicationServiceStateRecordField{},
		}
		if kind == assemblyline.ApplicationServiceStateRecordList {
			recordFields, err := resolveDirectCodingServiceRecordFields(
				runtime, model, subject+"_"+name, authority, name, identities,
			)
			if err != nil {
				return zero, err
			}
			field.RecordFields = recordFields
		}
		fields = append(fields, field)
	}
	result := assemblyline.ApplicationServiceStateInterfaceResult{
		Schema: assemblyline.ApplicationServiceStateInterfaceSchemaV1,
		Fields: fields,
	}
	if err := result.ValidateFor(authority); err != nil {
		return zero, err
	}
	return result, nil
}

func resolveDirectCodingServiceRecordFields(
	runtime typedWorkerRuntime,
	model string,
	subject string,
	authority assemblyline.ApplicationServiceStateInterfaceInput,
	parentName string,
	identities []assemblyline.ArtifactIdentity,
) ([]assemblyline.ApplicationServiceStateRecordField, error) {
	fields := make(
		[]assemblyline.ApplicationServiceStateRecordField, 0,
		assemblyline.MaxApplicationServiceStateInterfaceFields,
	)
	for {
		leafInput := assemblyline.ApplicationRecordFieldLeafInput{
			Authority: authority, ParentName: parentName,
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
				runtime, model, subject+"_record_field_coverage", coverageJob, identities,
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

		nameJob, err := assemblyline.NewApplicationRecordFieldNameJob(leafInput)
		if err != nil {
			return nil, err
		}
		name, err := runDirectCodingSemanticLeafCall(
			runtime, model, subject+"_record_field_name", nameJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationRecordFieldNameLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return nil, err
		}
		kindInput := assemblyline.ApplicationRecordFieldKindInput{
			Authority: authority, ParentName: parentName,
			AcceptedRecordFields: append(
				[]assemblyline.ApplicationServiceStateRecordField{}, fields...,
			),
			FocusedName: name,
		}
		kindJob, err := assemblyline.NewApplicationRecordFieldKindJob(kindInput)
		if err != nil {
			return nil, err
		}
		kind, err := runDirectCodingSemanticLeafCall(
			runtime, model, subject+"_record_field_kind", kindJob, identities,
			func(raw string) (assemblyline.ApplicationServiceStateFieldKind, error) {
				return assemblyline.DecodeApplicationRecordFieldKindLeaf(kindInput, raw)
			},
			func(assemblyline.ApplicationServiceStateFieldKind) error { return nil },
		)
		if err != nil {
			return nil, err
		}
		fields = append(fields, assemblyline.ApplicationServiceStateRecordField{
			Name: name, Kind: kind,
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
	workloadInput, err := applicationWorkloadInputFromFrozen(workload)
	if err != nil {
		return nil, err
	}
	requirements := make([]assemblyline.Requirement, 0, len(workload.Tasks))
	indexByRequirement := make(map[string]int, len(workload.Tasks))
	for index, task := range workload.Tasks {
		requirements = append(requirements, assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		})
		indexByRequirement[task.RequirementID] = index
	}
	if err := validateDirectCodingCapabilityGraph(requirements, capabilities); err != nil {
		return nil, err
	}
	adjacency := make([]map[int]struct{}, len(workload.Tasks))
	for index := range adjacency {
		adjacency[index] = make(map[int]struct{})
	}
	for ownerID, dependencies := range capabilities {
		owner := indexByRequirement[ownerID]
		for _, dependency := range dependencies {
			provider := indexByRequirement[dependency.RequirementID]
			adjacency[owner][provider] = struct{}{}
			adjacency[provider][owner] = struct{}{}
		}
	}
	visited := make([]bool, len(workload.Tasks))
	components := make([]directCodingServiceStateComponent, 0)
	for start := range workload.Tasks {
		if visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		indices := make([]int, 0)
		hasDurable := false
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			indices = append(indices, current)
			if plan.ByTask[workload.Tasks[current].ID] ==
				assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
				hasDurable = true
			}
			for candidate := range workload.Tasks {
				if _, linked := adjacency[current][candidate]; linked && !visited[candidate] {
					visited[candidate] = true
					queue = append(queue, candidate)
				}
			}
		}
		if !hasDurable {
			continue
		}
		component := directCodingServiceStateComponent{
			ID: fmt.Sprintf("state_interface_%03d", len(components)+1),
			Input: assemblyline.ApplicationServiceStateInterfaceInput{
				ProductContext: workload.ProductQuote,
			},
		}
		for _, index := range indices {
			task := workload.Tasks[index]
			authority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
				workloadInput, workload, task.ID,
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
		}
		if err := component.Input.Validate(); err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, nil
}
