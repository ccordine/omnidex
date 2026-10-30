package codingagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

// OrderingProvider supplies the existing lightweight semantic transport and its cleanup.
type OrderingProvider interface {
	OrderingClient(context.Context) (llm.ExactStationClient, func(), error)
}

// TaskOrderingEvidence binds a validated semantic permutation to its exact input
// and owner operation. The ledger transaction persists the resulting ranks.
type TaskOrderingEvidence struct {
	Key         string `json:"key"`
	OperationID string `json:"operation_id"`
	Route       Route  `json:"route"`
	Prompt      string `json:"prompt"`
	Response    string `json:"response"`
	ElapsedMS   int64  `json:"elapsed_ms"`
}

func (controller *Controller) orderPlanningTasks(ctx context.Context, turn PlanningTurn, tasks []LedgerTask) error {
	eligible := []LedgerTask{}
	for _, task := range tasks {
		if task.Disposition == model.LedgerDispositionActive {
			eligible = append(eligible, task)
		}
	}
	if len(eligible) < 2 {
		return nil
	}
	// Canonical input prevents parser slice order from invalidating a resolved order.
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	input := make([]struct {
		ID   model.LedgerItemID
		Goal string
	}, len(eligible))
	for i, task := range eligible {
		input[i].ID, input[i].Goal = task.ID, task.Goal
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%x", sha256.Sum256(raw))
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return err
	}
	var receipt *TaskOrderingEvidence
	for _, entry := range entries {
		if entry.TaskOrdering != nil && entry.TaskOrdering.Key == key {
			receipt = entry.TaskOrdering
		}
	}
	if receipt != nil {
		err = OrderUnorderedTasks(ctx, eligible, func(context.Context, string) (string, error) { return receipt.Response, nil })
	} else {
		route := controller.Configuration.LedgerParser
		source, ok := controller.Providers[route.Provider].(OrderingProvider)
		if !ok {
			return fmt.Errorf("ordering provider %q has no lightweight semantic transport", route.Provider)
		}
		provider, cleanup, transportErr := source.OrderingClient(ctx)
		if transportErr != nil {
			return transportErr
		}
		defer cleanup()
		receipt = &TaskOrderingEvidence{Key: key, OperationID: turn.OperationID, Route: route}
		err = OrderUnorderedTasks(ctx, eligible, func(ctx context.Context, prompt string) (string, error) {
			receipt.Prompt = prompt
			prepared, err := llm.PrepareNativeRequest(provider, llm.PreparedModel{Protocol: llm.ExactPreparedProtocolPlainCompletionV4, BaseModel: route.Model, ContextModel: route.Model, Prompt: prompt, MaxOutputTokens: 2048, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit, ContextTokens: 32768})
			if err != nil {
				return "", err
			}
			started := time.Now()
			result, err := provider.GeneratePreparedExact(ctx, prepared)
			receipt.ElapsedMS, receipt.Response = time.Since(started).Milliseconds(), result.Content
			if err != nil {
				return "", err
			}
			if err := llm.ValidateExactPreparedGenerationForRequest(prepared, result); err != nil {
				return "", err
			}
			return result.Content, nil
		})
		// Failed output is retained, but cannot become a reusable resolution.
		phase := "task_ordering"
		if err != nil {
			phase = "task_ordering_failed"
			receipt.Key = ""
		}
		if _, saveErr := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: phase, TaskOrdering: receipt}); saveErr != nil {
			return saveErr
		}
	}
	if err != nil {
		return err
	}
	ranks := map[model.LedgerItemID]int64{}
	for _, task := range eligible {
		ranks[task.ID] = task.Rank
	}
	for i := range tasks {
		if rank, ok := ranks[tasks[i].ID]; ok {
			tasks[i].Rank = rank
		}
	}
	return nil
}

// Existing owner-edited tasks keep their ranks. Only new unordered additions
// are ordered, in the new portion of the proposal; explicit reorders win.
func (controller *Controller) orderPlanningAdditions(ctx context.Context, current EngineeringLedger, turn PlanningTurn, mutation *LedgerMutation) error {
	for _, op := range mutation.Operations {
		if op.Kind == model.LedgerMutationReorder {
			return nil
		}
	}
	tasks := []LedgerTask{}
	var maximum int64
	for _, task := range current.Tasks {
		if task.Rank > maximum {
			maximum = task.Rank
		}
	}
	for _, op := range mutation.Operations {
		if op.Kind == model.LedgerMutationAdd && op.Item != nil && op.Item.Task != nil {
			tasks = append(tasks, *op.Item.Task)
		}
	}
	if len(tasks) == 0 {
		return nil
	}
	if err := controller.orderPlanningTasks(ctx, turn, tasks); err != nil {
		return err
	}
	ranks := map[model.LedgerItemID]int64{}
	for _, task := range tasks {
		ranks[task.ID] = maximum + task.Rank
		if len(tasks) == 1 {
			ranks[task.ID] = maximum + 1
		}
	}
	for i := range mutation.Operations {
		op := &mutation.Operations[i]
		if op.Kind == model.LedgerMutationAdd && op.Item != nil && op.Item.Task != nil {
			op.Item.Task.Rank = ranks[op.Item.Task.ID]
		}
	}
	return nil
}

// PublishPlanningLedger is called only with a committed ledger projection.
// Retain the planning conversation alongside the authoritative persisted order.
func (controller *Controller) PublishPlanningLedger(ctx context.Context, turn PlanningTurn, ledger EngineeringLedger) error {
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Phase == "planning_response" && len(entry.Notes) == 3 && entry.Notes[0] == turn.OperationID {
			return nil
		}
	}
	var discussion string
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Phase == "planning_draft" && len(entry.Notes) == 3 && entry.Notes[0] == turn.OperationID {
			discussion = entry.Notes[2]
			break
		}
	}
	tasks := append([]LedgerTask{}, ledger.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Rank < tasks[j].Rank || tasks[i].Rank == tasks[j].Rank && tasks[i].ID < tasks[j].ID
	})
	var text strings.Builder
	if discussion != "" {
		fmt.Fprintf(&text, "%s\n\n", discussion)
	}
	if turn.ItemID == "" {
		fmt.Fprintf(&text, "Proposal ledger %s, revision %d. Execution order:\n", ledger.ID, ledger.Revision)
	} else {
		fmt.Fprintf(&text, "Selected item · ledger revision %d:\n", ledger.Revision)
	}
	position := 0
	for _, task := range tasks {
		if turn.ItemID != "" && task.ID != turn.ItemID {
			continue
		}
		if task.Disposition == model.LedgerDispositionActive {
			position++
			fmt.Fprintf(&text, "\n%d. %s\n", position, task.Goal)
			if task.Objective != "" && task.Objective != task.Goal {
				fmt.Fprintf(&text, "   %s\n", task.Objective)
			}
		}
	}
	if position == 0 {
		// A clarification or discussion is still a useful planning reply. Do
		// not replace it with an empty execution-order heading.
		text.Reset()
		text.WriteString(discussion)
		if text.Len() == 0 {
			text.WriteString("There are no tasks to start yet. Continue planning with what you want done.")
		}
	}
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "planning_response", TaskID: string(turn.ItemID), Role: Plan, PlanningProposal: &ledger, Notes: []string{turn.OperationID, turn.Text, text.String()}})
	return err
}
