package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationTaskResultRelationBinding struct {
	TaskID        string
	RequirementID string
	Receipt       assemblyline.ApplicationRequirementCandidateResultRelationResult
}

// directCodingApplicationTaskResultRelationPlan is code-only validation
// authority. It is deliberately separate from the frozen workload and task
// context so no result-relation control value enters a model-visible task.
type directCodingApplicationTaskResultRelationPlan struct {
	WorkloadSHA256 string
	Bindings       []directCodingApplicationTaskResultRelationBinding
}

func newDirectCodingApplicationTaskResultRelationPlan(
	workload assemblyline.FrozenApplicationWorkload,
	accepted []assemblyline.ApplicationRequirement,
) (directCodingApplicationTaskResultRelationPlan, error) {
	var zero directCodingApplicationTaskResultRelationPlan
	if err := assemblyline.ValidateFrozenApplicationWorkload(workload); err != nil {
		return zero, err
	}
	if len(accepted) != len(workload.Tasks) {
		return zero, fmt.Errorf(
			"application task result-relation receipts do not cover frozen workload tasks",
		)
	}
	bindings := make([]directCodingApplicationTaskResultRelationBinding, len(workload.Tasks))
	requestSHA256 := ""
	for index, task := range workload.Tasks {
		requirement := accepted[index]
		if requirement.ID != task.RequirementID || requirement.Statement != task.RequirementQuote {
			return zero, fmt.Errorf(
				"application task %s result-relation authority differs from accepted requirement",
				task.ID,
			)
		}
		if requirement.RequestSHA256 == "" {
			return zero, fmt.Errorf(
				"application task %s result-relation authority lacks request provenance",
				task.ID,
			)
		}
		if requestSHA256 == "" {
			requestSHA256 = requirement.RequestSHA256
		} else if requirement.RequestSHA256 != requestSHA256 {
			return zero, fmt.Errorf(
				"application task %s result-relation authority differs from request provenance",
				task.ID,
			)
		}
		if err := requirement.ResultRelation.ValidateAcceptedFor(task.RequirementQuote); err != nil {
			return zero, fmt.Errorf(
				"application task %s result relation: %w", task.ID, err,
			)
		}
		bindings[index] = directCodingApplicationTaskResultRelationBinding{
			TaskID: task.ID, RequirementID: task.RequirementID,
			Receipt: requirement.ResultRelation,
		}
	}
	plan := directCodingApplicationTaskResultRelationPlan{
		WorkloadSHA256: workload.SHA256,
		Bindings:       bindings,
	}
	if err := plan.validateCompleteFor(workload); err != nil {
		return zero, err
	}
	return plan, nil
}

func (plan directCodingApplicationTaskResultRelationPlan) validateCompleteFor(
	workload assemblyline.FrozenApplicationWorkload,
) error {
	if err := assemblyline.ValidateFrozenApplicationWorkload(workload); err != nil {
		return err
	}
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != workload.SHA256 {
		return fmt.Errorf("application task result-relation plan differs from frozen workload authority")
	}
	if len(plan.Bindings) != len(workload.Tasks) {
		return fmt.Errorf("application task result-relation plan does not cover frozen workload")
	}
	for index, task := range workload.Tasks {
		if err := validateDirectCodingApplicationTaskResultRelationBinding(
			plan.Bindings[index], task,
		); err != nil {
			return err
		}
	}
	return nil
}

func (plan directCodingApplicationTaskResultRelationPlan) projectTask(
	workload assemblyline.FrozenApplicationWorkload,
	taskID string,
) (directCodingApplicationTaskResultRelationPlan, error) {
	var zero directCodingApplicationTaskResultRelationPlan
	if err := plan.validateCompleteFor(workload); err != nil {
		return zero, err
	}
	for index, task := range workload.Tasks {
		if task.ID != taskID {
			continue
		}
		return directCodingApplicationTaskResultRelationPlan{
			WorkloadSHA256: plan.WorkloadSHA256,
			Bindings: []directCodingApplicationTaskResultRelationBinding{
				plan.Bindings[index],
			},
		}, nil
	}
	return zero, fmt.Errorf("application task result-relation projection task %q is unknown", taskID)
}

func (plan directCodingApplicationTaskResultRelationPlan) bindingForTask(
	workload assemblyline.FrozenApplicationWorkload,
	taskID string,
) (directCodingApplicationTaskResultRelationBinding, error) {
	var zero directCodingApplicationTaskResultRelationBinding
	if err := assemblyline.ValidateFrozenApplicationWorkload(workload); err != nil {
		return zero, err
	}
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != workload.SHA256 {
		return zero, fmt.Errorf("application task result-relation plan differs from frozen workload authority")
	}
	if len(plan.Bindings) == len(workload.Tasks) {
		if err := plan.validateCompleteFor(workload); err != nil {
			return zero, err
		}
		for index, task := range workload.Tasks {
			if task.ID == taskID {
				return plan.Bindings[index], nil
			}
		}
		return zero, fmt.Errorf("application task result-relation task %q is unknown", taskID)
	}
	if len(plan.Bindings) != 1 {
		return zero, fmt.Errorf("application task result-relation stage requires exactly one binding")
	}
	for _, task := range workload.Tasks {
		if task.ID != taskID {
			continue
		}
		binding := plan.Bindings[0]
		if err := validateDirectCodingApplicationTaskResultRelationBinding(binding, task); err != nil {
			return zero, err
		}
		return binding, nil
	}
	return zero, fmt.Errorf("application task result-relation stage task %q is unknown", taskID)
}

func validateDirectCodingApplicationTaskResultRelationBinding(
	binding directCodingApplicationTaskResultRelationBinding,
	task assemblyline.FrozenApplicationTask,
) error {
	if binding.TaskID != task.ID || binding.RequirementID != task.RequirementID {
		return fmt.Errorf(
			"application task %s result-relation binding differs from frozen task authority",
			task.ID,
		)
	}
	if err := binding.Receipt.ValidateAcceptedFor(task.RequirementQuote); err != nil {
		return fmt.Errorf("application task %s result relation: %w", task.ID, err)
	}
	return nil
}
