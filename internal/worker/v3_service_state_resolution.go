package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingServiceStatePlan struct {
	WorkloadSHA256  string
	ByTask          map[string]assemblyline.ApplicationServiceStateLifetime
	Interfaces      []directCodingServiceStateInterfaceBinding
	InterfaceByTask map[string]string
}

func (s *directCodingSession) resolveServiceStateBeforeTargetTree(
	runtime typedWorkerRuntime,
	stack directCodingProjectStack,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceStatePlan, error) {
	if stack.CompileServiceSource == nil {
		return directCodingServiceStatePlan{}, nil
	}
	if stack.ValidateServiceState == nil {
		return directCodingServiceStatePlan{}, fmt.Errorf(
			"HTTP project stack %s has no executable state-lifetime validator", stack.ID,
		)
	}
	model, err := s.workerModel(station.CodingServiceStateLifetime)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	plan, err := resolveDirectCodingServiceStatePlan(
		runtime, model, applicationWorkloadInput(specification), workload, identities,
	)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	if err := stack.ValidateServiceState(workload, plan); err != nil {
		return directCodingServiceStatePlan{}, fmt.Errorf(
			"project stack %s rejected service state authority before target-tree resolution: %w",
			stack.ID, err,
		)
	}
	return plan, nil
}

func resolveDirectCodingServiceStatePlan(
	runtime typedWorkerRuntime,
	model string,
	workloadInput assemblyline.ApplicationWorkloadDraftInput,
	workload assemblyline.FrozenApplicationWorkload,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceStatePlan, error) {
	if workloadInput.ProductQuote == "" || workloadInput.ProductQuote != workload.ProductQuote {
		return directCodingServiceStatePlan{}, fmt.Errorf(
			"service state resolution requires the frozen HTTP product authority",
		)
	}
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	plan := directCodingServiceStatePlan{
		WorkloadSHA256: workload.SHA256,
		ByTask: make(
			map[string]assemblyline.ApplicationServiceStateLifetime,
			len(workload.Tasks),
		),
	}
	for _, task := range workload.Tasks {
		authority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
			workloadInput, workload, task.ID,
		)
		if err != nil {
			return directCodingServiceStatePlan{}, err
		}
		input, err := assemblyline.ProjectApplicationServiceStateLifetimeInput(
			authority,
		)
		if err != nil {
			return directCodingServiceStatePlan{}, err
		}
		job, err := assemblyline.NewApplicationServiceStateLifetimeJob(input)
		if err != nil {
			return directCodingServiceStatePlan{}, err
		}
		result, err := runDirectCodingSemanticLeafCall(
			runtime, model, "application_service_state_lifetime", job, identities,
			func(raw string) (assemblyline.ApplicationServiceStateLifetimeResult, error) {
				return assemblyline.DecodeApplicationServiceStateLifetimeResult(input, raw)
			},
			func(value assemblyline.ApplicationServiceStateLifetimeResult) error {
				return value.ValidateFor(input)
			},
		)
		if err != nil {
			return directCodingServiceStatePlan{}, fmt.Errorf(
				"resolve service state lifetime for task %s: %w", task.ID, err,
			)
		}
		plan.ByTask[task.ID] = result.StateLifetime
	}
	if err := plan.ValidateFor(workload); err != nil {
		return directCodingServiceStatePlan{}, err
	}
	return plan, nil
}

func (plan directCodingServiceStatePlan) ValidateFor(
	workload assemblyline.FrozenApplicationWorkload,
) error {
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != workload.SHA256 {
		return fmt.Errorf("service state plan differs from frozen workload authority")
	}
	if len(plan.ByTask) != len(workload.Tasks) {
		return fmt.Errorf(
			"service state plan has %d lifetime decisions for %d frozen tasks",
			len(plan.ByTask), len(workload.Tasks),
		)
	}
	known := make(map[string]struct{}, len(workload.Tasks))
	for _, task := range workload.Tasks {
		known[task.ID] = struct{}{}
		lifetime, exists := plan.ByTask[task.ID]
		if !exists {
			return fmt.Errorf("service state plan omits frozen task %s", task.ID)
		}
		switch lifetime {
		case assemblyline.ApplicationServiceStateRequestLocalOnly,
			assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired:
		default:
			return fmt.Errorf(
				"service state plan task %s has unsupported lifetime %q", task.ID, lifetime,
			)
		}
	}
	for taskID := range plan.ByTask {
		if _, exists := known[taskID]; !exists {
			return fmt.Errorf("service state plan contains unknown task %s", taskID)
		}
	}
	return nil
}

func (plan directCodingServiceStatePlan) projectTask(
	taskID string,
) (directCodingServiceStatePlan, error) {
	lifetime, exists := plan.ByTask[taskID]
	if !exists {
		return directCodingServiceStatePlan{}, fmt.Errorf(
			"service state plan omits projected task %s", taskID,
		)
	}
	return directCodingServiceStatePlan{
		WorkloadSHA256: plan.WorkloadSHA256,
		ByTask: map[string]assemblyline.ApplicationServiceStateLifetime{
			taskID: lifetime,
		},
		Interfaces:      projectedServiceStateInterfaces(plan, taskID),
		InterfaceByTask: projectedServiceStateInterfaceTasks(plan, taskID),
	}, nil
}

func projectedServiceStateInterfaces(
	plan directCodingServiceStatePlan,
	taskID string,
) []directCodingServiceStateInterfaceBinding {
	interfaceID, exists := plan.InterfaceByTask[taskID]
	if !exists {
		return nil
	}
	for _, binding := range plan.Interfaces {
		if binding.ID == interfaceID {
			return []directCodingServiceStateInterfaceBinding{binding}
		}
	}
	return nil
}

func projectedServiceStateInterfaceTasks(
	plan directCodingServiceStatePlan,
	taskID string,
) map[string]string {
	interfaceID, exists := plan.InterfaceByTask[taskID]
	if !exists {
		return nil
	}
	return map[string]string{taskID: interfaceID}
}
