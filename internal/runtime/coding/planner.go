package coding

import (
	"context"
	"strings"
)

type identityInterpreter struct{}

func (identityInterpreter) Interpret(_ context.Context, req Request) (Request, error) {
	req.Goal = strings.TrimSpace(req.Goal)
	return req, nil
}

type deterministicPlanner struct{}

func (deterministicPlanner) Plan(_ context.Context, req Request) (CodingPlan, error) {
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		goal = "complete coding task"
	}
	return CodingPlan{
		Goal: goal,
		Tasks: []CodingPlannerTask{{
			ID:              "task_1",
			Objective:       goal,
			SuccessCriteria: []string{"requested coding workflow completed"},
		}},
	}, nil
}

func (deterministicPlanner) Disposition(_ context.Context, _ CodingPlan, report EmptyFileReport) (PlannerDisposition, error) {
	actions := make([]PlannerDispositionAction, 0, len(report.Files))
	for _, file := range report.Files {
		actions = append(actions, PlannerDispositionAction{
			Path:   file,
			Action: "ignore",
			Reason: "deterministic planner leaves empty files unchanged without explicit task scope",
		})
	}
	return PlannerDisposition{Complete: true, Actions: actions}, nil
}
