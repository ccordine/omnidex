package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
)

type ExecutionResponseKind string

const (
	ExecutionDone       ExecutionResponseKind = "done"
	ExecutionQuestion   ExecutionResponseKind = "question"
	ExecutionPermission ExecutionResponseKind = "permission"
	ExecutionError      ExecutionResponseKind = "error"
)

// ExecutionInputRequired is a code-owned routing result, not executor output.
type ExecutionInputRequired struct {
	TaskID string
	Role   Role
	Kind   ExecutionResponseKind
	Text   string
}

func (request *ExecutionInputRequired) Error() string { return request.Text }

func (controller *Controller) handleExecutionResponse(ctx context.Context, task Task, result Result, round int64) error {
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil {
		return fmt.Errorf("decode ordinary Codex response: %w", err)
	}
	call := controller.assignment(ExecutionResponse, nil, controller.Configuration.LedgerParser)
	call.Response = response
	parsed, err := controller.invoke(ctx, call, round)
	if err != nil {
		return err
	}
	var outcome struct {
		Kind ExecutionResponseKind `json:"kind"`
	}
	if err := decodeContract(parsed.Content, &outcome); err != nil {
		return &ParserFailure{Cause: err}
	}
	switch outcome.Kind {
	case ExecutionDone, ExecutionQuestion, ExecutionPermission, ExecutionError:
	default:
		return &ParserFailure{Cause: fmt.Errorf("execution response parser returned unknown kind %q", outcome.Kind)}
	}
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "execution_response",
		TaskID: task.ID, CallID: result.CallID, Round: round, ResponseKind: outcome.Kind, Notes: []string{response}})
	if err != nil {
		return err
	}
	switch outcome.Kind {
	case ExecutionQuestion, ExecutionPermission:
		return &ExecutionInputRequired{TaskID: task.ID, Kind: outcome.Kind, Text: response}
	case ExecutionError:
		return &IncompleteTaskError{Reason: "Execution is incomplete: " + response, ThreadID: result.ThreadID, TurnID: result.TurnID}
	default:
		return nil
	}
}
