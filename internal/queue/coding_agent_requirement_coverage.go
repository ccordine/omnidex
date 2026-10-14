package queue

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func validateCodingAgentRequirementCoverage(entries []codingagent.Entry, plan model.CodingPlan) error {
	tasks := map[string]codingagent.Task{}
	opened := map[int64]codingagent.Entry{}
	var workload *codingagent.Workload
	var planning *codingagent.Assignment
	for _, entry := range entries {
		if entry.Phase == "call_opened" {
			opened[entry.ID] = entry
		}
		if entry.Phase != "plan_frozen" || entry.Workload == nil {
			continue
		}
		if workload != nil {
			return fmt.Errorf("agent completion contains multiple frozen workloads")
		}
		workload = entry.Workload
		call, known := opened[entry.CallID]
		if !known || call.Assignment == nil || call.Assignment.Role != codingagent.Plan {
			return fmt.Errorf("agent workload lacks its exact planning context")
		}
		planning = call.Assignment
		for _, task := range workload.Tasks {
			if len(task.RequirementIDs) == 0 {
				return fmt.Errorf("agent task lacks its frozen requirement binding")
			}
			tasks[task.ID] = task
		}
	}
	if workload == nil || planning == nil || planning.Objective == nil || len(planning.Requirements) == 0 {
		return fmt.Errorf("agent completion omitted its approved objective or workload")
	}
	requirements := map[string]string{}
	for _, requirement := range planning.Requirements {
		if requirement.ID == "" || requirements[requirement.ID] != "" {
			return fmt.Errorf("agent planning contains invalid parent requirement identity")
		}
		requirements[requirement.ID] = requirement.Outcome
	}
	covered := map[string]bool{}
	for _, task := range workload.Tasks {
		for _, id := range task.RequirementIDs {
			if _, known := requirements[id]; !known {
				return fmt.Errorf("agent task introduced an unapproved requirement")
			}
			covered[id] = true
		}
	}
	if len(covered) != len(requirements) || len(plan.Leaves) != len(workload.Tasks) {
		return fmt.Errorf("agent completion omitted approved objective coverage")
	}
	for index, leaf := range plan.Leaves {
		statement, err := codingagent.PlanTaskStatement(workload.Tasks[index])
		if err != nil {
			return err
		}
		if leaf.Decision != model.CodingPlanDecisionApproved || leaf.Statement != statement {
			return fmt.Errorf("agent frozen workload differs from the user-approved plan")
		}
	}
	for _, entry := range entries {
		var expected map[string]string
		switch entry.Phase {
		case "plan_frozen", "overall_accepted":
			expected = requirements
		case "task_accepted":
			task, known := tasks[entry.TaskID]
			if !known {
				return fmt.Errorf("agent acceptance refers to an unknown task")
			}
			expected = map[string]string{}
			for _, id := range task.RequirementIDs {
				expected[id] = requirements[id]
			}
		default:
			continue
		}
		call, known := opened[entry.CallID]
		if !known || call.Assignment == nil {
			return fmt.Errorf("agent acceptance lacks its frozen requirement context")
		}
		if entry.Phase != "task_accepted" {
			if err := codingAgentRequirementContextMatches(call.Assignment.Requirements, expected); err != nil {
				return err
			}
		}
		if entry.Phase == "task_accepted" {
			if err := call.Assignment.ValidateTaskContext(); err != nil {
				return err
			}
			task := tasks[entry.TaskID]
			if call.Assignment.Task == nil || !reflect.DeepEqual(*call.Assignment.Task, task) {
				return fmt.Errorf("agent accepted review changed the frozen narrow task")
			}
		}
		if entry.Phase == "overall_accepted" && (call.Assignment.Workload == nil || !reflect.DeepEqual(*call.Assignment.Workload, *workload)) {
			return fmt.Errorf("agent overall review changed the frozen workload")
		}
		if entry.Phase == "overall_accepted" && !reflect.DeepEqual(call.Assignment.Objective, planning.Objective) {
			return fmt.Errorf("agent overall review changed the approved objective")
		}
	}
	return nil
}

func codingAgentRequirementContextMatches(actual []codingagent.Requirement, expected map[string]string) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("agent call changed its frozen parent requirement context")
	}
	seen := map[string]bool{}
	for _, requirement := range actual {
		outcome, known := expected[requirement.ID]
		if !known || seen[requirement.ID] || outcome != requirement.Outcome {
			return fmt.Errorf("agent call changed its exact parent requirement identity or outcome")
		}
		seen[requirement.ID] = true
	}
	return nil
}
