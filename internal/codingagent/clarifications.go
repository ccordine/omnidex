package codingagent

import "context"

type TaskClarification struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func (controller *Controller) taskClarifications(ctx context.Context, taskID string) ([]TaskClarification, error) {
	return controller.Evidence.ReadTaskClarifications(ctx, taskID)
}

func ExtractTaskClarifications(entries []Entry, taskID string) []TaskClarification {
	questions := map[int64]string{}
	answers := []TaskClarification{}
	for _, entry := range entries {
		if entry.TaskID != taskID {
			continue
		}
		switch entry.Phase {
		case "execution_native_input":
			if entry.NativeInput != nil && entry.NativeInput.Method == "item/tool/requestUserInput" && len(entry.Notes) == 1 {
				questions[entry.CallID] = entry.Notes[0]
			}
		case "execution_response", "review_response":
			if len(entry.Notes) == 1 && (entry.NativeInput != nil || entry.ResponseKind == ExecutionQuestion || entry.ResponseKind == ExecutionPermission) {
				questions[entry.CallID] = entry.Notes[0]
			}
		case "execution_reply":
			if question := questions[entry.CallID]; question != "" && len(entry.Notes) == 2 {
				answers = append(answers, TaskClarification{Question: question, Answer: entry.Notes[1]})
			}
		}
	}
	return answers
}
