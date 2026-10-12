package codingagent

import "fmt"

func bindWorkloadRequirements(workload *Workload, requirements []Requirement) error {
	known := map[string]string{}
	covered := map[string]bool{}
	for _, requirement := range requirements {
		known[requirement.ID] = requirement.Outcome
	}
	for _, task := range workload.Tasks {
		if len(task.RequirementIDs) == 0 {
			return fmt.Errorf("task %s has no frozen requirement binding", task.ID)
		}
		for _, id := range task.RequirementIDs {
			_, exists := known[id]
			if !exists {
				return fmt.Errorf("task %s introduced an unknown requirement", task.ID)
			}
			covered[id] = true
		}
	}
	if len(covered) != len(known) {
		return fmt.Errorf("agent plan omitted a frozen requirement")
	}
	return nil
}

func unansweredQuestions(entries []Entry) []string {
	var questions []string
	var questionID int64
	for _, entry := range entries {
		switch entry.Phase {
		case "questions":
			questions = entry.Questions
			questionID = entry.ID
		case "answers":
			if entry.QuestionID == questionID && questionID != 0 {
				questions = nil
			}
		}
	}
	return questions
}
