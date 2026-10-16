package queue

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
)

// The reviewer stays ordinary Codex. Acceptance binds its exact prose to a
// separate completed parser call on the frozen configured parser route.
func codingAgentAcceptedReviewResponse(accepted codingagent.Entry, route codingagent.Route,
	opened, completed map[int64]codingagent.Entry, native map[int64]bool, entries []codingagent.Entry,
) error {
	var response string
	if err := decodeCodingAgentJournalResult(completed[accepted.CallID].Result.Content, &response); err != nil || strings.TrimSpace(response) == "" {
		return fmt.Errorf("agent review lacks its ordinary final response")
	}
	var receipt *codingagent.Entry
	for index := range entries {
		entry := &entries[index]
		if entry.Phase == "review_response" && entry.CallID == accepted.CallID && entry.ID < accepted.ID {
			receipt = entry
		}
	}
	if receipt == nil || receipt.TaskID != accepted.TaskID || receipt.ResponseKind != codingagent.ExecutionDone ||
		receipt.ReviewResponse == nil || len(receipt.Notes) != 1 || receipt.Notes[0] != response {
		return fmt.Errorf("agent acceptance lacks a finished parsed review response")
	}
	parserAcceptance := *receipt
	parserAcceptance.CallID = receipt.ParserCallID
	if err := codingAgentAcceptedCall(parserAcceptance, codingagent.ReviewResponse, route, opened, completed, native); err != nil {
		return fmt.Errorf("agent acceptance lacks its review response parser: %w", err)
	}
	parser := opened[receipt.ParserCallID].Assignment
	task := opened[accepted.CallID].Assignment.Task
	if task == nil || parser.Response != response || !reflect.DeepEqual(parser.Task, task) || parser.ValidateTaskContext() != nil ||
		receipt.ParserCallID <= completed[accepted.CallID].ID {
		return fmt.Errorf("review response parser changed its exact source response or task")
	}
	var parsed codingagent.ReviewResponseResult
	if err := decodeCodingAgentJournalResult(completed[receipt.ParserCallID].Result.Content, &parsed); err != nil {
		return err
	}
	if !reflect.DeepEqual(parsed, *receipt.ReviewResponse) || parsed.Kind != codingagent.ExecutionDone ||
		len(parsed.Findings) != 0 || codingagent.ValidateReview(map[string]codingagent.Task{task.ID: *task}, parsed.ReviewResult) != nil {
		return fmt.Errorf("agent acceptance contains unresolved review findings or lacks verification")
	}
	return nil
}
