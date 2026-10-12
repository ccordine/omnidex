package queue

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func validateCodingAgentRequirementCoverage(entries []codingagent.Entry, plan model.CodingPlan) error {
	requirements := map[string]string{}
	for _, leaf := range plan.Leaves {
		if leaf.Decision == model.CodingPlanDecisionApproved {
			requirements[string(leaf.ID)] = leaf.Statement
		}
	}
	covered := map[string]bool{}
	tasks := map[string]codingagent.Task{}
	opened := map[int64]codingagent.Entry{}
	var workload *codingagent.Workload
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
		for _, task := range workload.Tasks {
			if len(task.RequirementIDs) == 0 {
				return fmt.Errorf("agent task lacks its frozen requirement binding")
			}
			tasks[task.ID] = task
			for _, id := range task.RequirementIDs {
				if _, known := requirements[id]; !known {
					return fmt.Errorf("agent task introduced an unapproved requirement")
				}
				covered[id] = true
			}
		}
	}
	if len(requirements) == 0 || len(covered) != len(requirements) || workload == nil {
		return fmt.Errorf("agent completion omitted frozen requirements")
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
		if entry.Phase == "plan_frozen" && call.Assignment.Objective != nil {
			return fmt.Errorf("agent task planner received an unapproved original objective")
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
