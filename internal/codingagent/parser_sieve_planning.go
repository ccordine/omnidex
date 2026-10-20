package codingagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type parserPlanningProjection struct {
	Operations []planningParserOperation `json:"operations"`
}

// Provider adapters return validated, materialized operations from the sieve.
// Sparse model patches are decoded only inside the sieve, before this boundary.
type planningSieveMutationResult struct {
	Operations []model.LedgerMutationOperation `json:"operations"`
}

func (s *parserSieve) planning(ctx context.Context) (json.RawMessage, error) {
	current, err := parserPlanningLedger(s.call)
	if err != nil {
		return nil, err
	}
	operations := []model.LedgerMutationOperation{}
	instructions := []model.TaskContext{}
	unit := 0
	var owningTask model.LedgerItemID
	sourceHash := fmt.Sprintf("%x", sha256.Sum256([]byte(s.call.PlanningResponse)))[:12]
	for index, parts := range s.parts {
		for _, part := range parts {
			if part.Kind == "ignore" {
				continue
			}
			if part.Kind == "project_instruction" {
				instructions = append(instructions, model.TaskContext{Text: strings.TrimSpace(part.Text), Provenance: model.LedgerProvenance{Kind: "final_response_statement", Reference: sourceHash + ":" + s.blocks[index].ID, Revision: current.Revision}})
				continue
			}
			unit++
			step := s.step(index, "planning_unit")
			step.Source, step.Category = part.Text, part.Kind
			projectionLedger := planningParserContext(current, part, owningTask, s.call.CompletedTaskIDs)
			step.Ledger = &projectionLedger
			if part.Kind == "change" {
				if _, err := renderParserStepContext(Assignment{ParserStep: &step}); err != nil {
					selected, err := s.resolvePlanningTargets(ctx, step, projectionLedger)
					if err != nil {
						return nil, err
					}
					step.Ledger = &selected
				}
			}
			step.NewIDs = map[string]string{"task": fmt.Sprintf("p_%s_%d_task", sourceHash, unit)}
			if part.Kind == "task_support" {
				if owningTask == "" {
					return nil, fmt.Errorf("task support has no owning task")
				}
				step.NewIDs = map[string]string{}
				step.Before = "Supporting instructions belong only to task " + string(owningTask)
			}
			originalGoal := ""
			if part.Kind == "task_support" {
				originalGoal = retainedPlanningItem(current, owningTask).Task.Goal
			}
			if _, err := renderParserStepContext(Assignment{ParserStep: &step}); err != nil {
				return nil, err
			}
			var projection parserPlanningProjection
			if err := s.leaf(ctx, step, &projection); err != nil {
				return nil, err
			}
			raw, _ := json.Marshal(projection)
			normalized, err := normalizeParserPlanningItems(raw, part, step, current)
			if err != nil {
				return nil, fmt.Errorf("%s planning statement: %w", step.BlockID, err)
			}
			parsed, err := decodePlanningOperations(normalized, current)
			if err != nil {
				return nil, err
			}
			if part.Kind == "task_support" {
				for _, op := range parsed {
					if op.Kind != model.LedgerMutationAmend || op.TargetID != owningTask || op.Item == nil || op.Item.Task == nil {
						return nil, fmt.Errorf("supporting instructions may amend only their owning task %s", owningTask)
					}
				}
			}
			for opIndex := range parsed {
				op := parsed[opIndex]
				if op.Kind != model.LedgerMutationAdd && retainedPlanningItem(*step.Ledger, op.TargetID) == nil {
					return nil, fmt.Errorf("planning edit target %s was not in the supplied packets", op.TargetID)
				}
				if part.Kind == "task_support" {
					merged, err := mergePlanningSupport(*retainedPlanningItem(current, owningTask).Task, *op.Item.Task)
					if err != nil {
						return nil, err
					}
					op.Item = &model.LedgerMutationItem{Task: &merged}
					parsed[opIndex] = op
				}
				if part.Kind != "task_support" && op.Item != nil && op.Item.Task != nil {
					owningTask = op.Item.Task.ID
				}
				if err := applyPlanningOperation(&current, op); err != nil {
					return nil, err
				}
			}
			if part.Kind == "task_support" {
				task := *retainedPlanningItem(current, owningTask).Task
				task.Goal = originalGoal
				// Keep the original support span even when a sparse semantic
				// projection extracts just one file or criterion from it.
				if !strings.Contains(task.Goal, strings.TrimSpace(part.Text)) {
					task.Goal += "\n\n" + strings.TrimSpace(part.Text)
				}
				op := model.LedgerMutationOperation{Kind: model.LedgerMutationAmend, TargetID: owningTask, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "Retain the owning task's supporting source."}
				if err := applyPlanningOperation(&current, op); err != nil {
					return nil, err
				}
				parsed = append(parsed, op)
			}
			// Supporting paragraphs may complete a criterion in a later span.
			// Validate the complete projection before returning it below.
			for _, op := range parsed {
				// Consecutive source spans can build one packet incrementally.
				// Only its complete final value crosses the mutation boundary.
				if len(operations) > 0 && op.Kind == model.LedgerMutationAmend && op.Item != nil && op.Item.Task != nil {
					last := &operations[len(operations)-1]
					if last.Item != nil && last.Item.Task != nil && last.Item.Task.ID == op.TargetID && (last.Kind == model.LedgerMutationAdd || last.Kind == model.LedgerMutationAmend) {
						last.Item = op.Item
						continue
					}
				}
				operations = append(operations, op)
			}
		}
	}
	// Shared user instructions are ordinary task text, not separately approved records.
	for _, task := range append([]model.LedgerTask(nil), current.Tasks...) {
		if task.Disposition != model.LedgerDispositionActive || contains(s.call.CompletedTaskIDs, string(task.ID)) {
			continue
		}
		context := appendDistinct(task.OwnerContext, instructions)
		if len(context) == len(task.OwnerContext) {
			continue
		}
		task.OwnerContext = context
		op := model.LedgerMutationOperation{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "Retain the stated shared instructions with this task."}
		if err := applyPlanningOperation(&current, op); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	if err := current.ValidateDraft(); err != nil {
		return nil, err
	}
	if s.call.Role == PlanningInitial {
		return json.Marshal(map[string]any{"ledger": current})
	}
	return json.Marshal(planningSieveMutationResult{Operations: operations})
}

func parserPlanningLedger(call Assignment) (EngineeringLedger, error) {
	ledger := EngineeringLedger{ID: fmt.Sprintf("planning-%d", call.JobID), Revision: max(1, call.PlanningBaseRevision)}
	if call.Role == PlanningMutation {
		found := false
		for _, fact := range call.ConstructionFacts {
			if fact.Kind != "bounded_planning_context" {
				continue
			}
			var payload struct {
				Ledger *EngineeringLedger `json:"ledger"`
			}
			if err := json.Unmarshal([]byte(fact.Quote), &payload); err != nil {
				return ledger, err
			}
			if payload.Ledger != nil {
				ledger, found = *payload.Ledger, true
			}
		}
		if !found && call.PlanningLedger != nil {
			ledger = *call.PlanningLedger
			found = true
		}
		if !found {
			return ledger, fmt.Errorf("planning mutation sieve requires the exact current ledger")
		}
	}
	// Clone, so previews and consumer bindings never mutate the caller's draft.
	raw, err := json.Marshal(ledger)
	if err != nil {
		return ledger, err
	}
	// Unmarshal must not reuse the caller's task slice backing array.
	ledger = EngineeringLedger{}
	err = json.Unmarshal(raw, &ledger)
	if err == nil {
		err = ledger.ValidateDraft()
	}
	return ledger, err
}

func normalizeParserPlanningItems(raw json.RawMessage, part ParserPart, step ParserStep, ledger EngineeringLedger) (json.RawMessage, error) {
	var parsed struct {
		Operations []planningParserOperation `json:"operations"`
	}
	if err := decodeContract(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed.Operations == nil {
		return nil, fmt.Errorf("planning unit requires operations")
	}
	rank := int64(0)
	for _, item := range planningItemPointers(&ledger) {
		rank = max(rank, item.Rank)
	}
	seen := map[string]bool{}
	for index := range parsed.Operations {
		op := &parsed.Operations[index]
		if op.Kind != model.LedgerMutationAdd {
			continue
		}
		var wrapper map[string]map[string]json.RawMessage
		if err := json.Unmarshal(op.Item, &wrapper); err != nil || len(wrapper) != 1 {
			return nil, fmt.Errorf("new parser item needs one typed semantic payload")
		}
		var kind string
		for name, fields := range wrapper {
			kind = name
			for _, owned := range []string{"provenance", "rank", "disposition", "review_state", "rationale"} {
				if fields[owned] != nil {
					return nil, fmt.Errorf("parser reconstructed code-owned field %s", owned)
				}
			}
		}
		if !contains(planningUnitItemKinds(part.Kind), kind) || seen[kind] {
			return nil, fmt.Errorf("planning unit added an unsupported or repeated item kind %s", kind)
		}
		seen[kind] = true
		var item model.LedgerMutationItem
		if err := decodeContract(op.Item, &item); err != nil {
			return nil, err
		}
		base := item.Base()
		if base == nil || string(base.ID) != step.NewIDs[kind] {
			return nil, fmt.Errorf("new item did not use its code-selected identity")
		}
		rank++
		base.Provenance = model.LedgerProvenance{Kind: "final_response_statement", Reference: step.BlockID + ":" + string(base.ID), Revision: ledger.Revision}
		base.Rank, base.Disposition, base.ReviewState = rank, model.LedgerDispositionActive, model.LedgerReviewProposed
		// Every added obligation retains its entire admitted source span. The
		// projection supplies relationships, never a lossy replacement summary.
		if item.Task == nil {
			return nil, fmt.Errorf("planning may add only tasks")
		}
		item.Task.Goal = strings.TrimSpace(part.Text)
		for _, list := range [][]string{item.Task.ReadFiles, item.Task.WriteFiles, item.Task.DeleteFiles} {
			for _, file := range list {
				if !strings.Contains(part.Text, file) {
					return nil, fmt.Errorf("parser invented a file absent from its source statement: %s", file)
				}
			}
		}

		op.Item, _ = json.Marshal(item)
	}
	return json.Marshal(parsed)
}

// Projection never changes the authoritative ledger used to materialize patches.
// An explicit task ID or a supporting span needs only its owning packet. Other
// statements receive active task identities and their complete source instructions
// and structured obligations. Oversized candidate sets resolve references before
// extracting changes against complete selected packets;
// collection fields must be visible because sparse edits replace whole collections.
// Retired source stays durable in the ledger.
func planningParserContext(ledger EngineeringLedger, part ParserPart, owner model.LedgerItemID, completed []string) EngineeringLedger {
	out := EngineeringLedger{ID: ledger.ID, Revision: ledger.Revision, Tasks: []LedgerTask{}}
	explicit := map[model.LedgerItemID]bool{}
	words := strings.FieldsFunc(part.Text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.:-", r))
	})
	for _, task := range ledger.Tasks {
		if contains(words, string(task.ID)) || contains(words, string(task.ID)+".") {
			explicit[task.ID] = true
		}
	}
	for _, task := range ledger.Tasks {
		if part.Kind == "task_support" {
			if task.ID == owner {
				out.Tasks = append(out.Tasks, task)
			}
			continue
		}
		if len(explicit) > 0 {
			if explicit[task.ID] {
				out.Tasks = append(out.Tasks, task)
			}
			continue
		}
		if task.Disposition == model.LedgerDispositionActive && !contains(completed, string(task.ID)) {
			out.Tasks = append(out.Tasks, task)
		}
	}
	return out
}

// Reference resolution cannot mutate task fields or select queue order. It only
// binds a descriptive source reference to existing IDs before full packet lookup.
type planningTaskReference struct {
	ID        model.LedgerItemID `json:"id"`
	Rank      int64              `json:"rank"`
	Goal      string             `json:"goal"`
	Objective string             `json:"objective,omitempty"`
}

// Each option ID belongs to one round only; accepted choices never reappear.
func (s *parserSieve) resolvePlanningTargets(ctx context.Context, step ParserStep, ledger EngineeringLedger) (EngineeringLedger, error) {
	selected := EngineeringLedger{ID: ledger.ID, Revision: ledger.Revision, Tasks: []LedgerTask{}}
	remaining := append([]LedgerTask(nil), ledger.Tasks...)
	if len(remaining) == 1 {
		selected.Tasks = append(selected.Tasks, remaining[0])
		return selected, nil
	}
	for round := 0; len(remaining) > 0; round++ {
		selection := step
		selection.Stage, selection.Ledger, selection.TaskIndex = "planning_targets", nil, nil
		choices := map[string]int{}
		for index, task := range remaining {
			id := fmt.Sprintf("choice_%d_%d", round, index)
			choices[id] = index
			selection.TaskIndex = append(selection.TaskIndex, planningTaskReference{ID: model.LedgerItemID(id), Rank: task.Rank, Goal: task.Goal, Objective: task.Objective})
		}
		stop := fmt.Sprintf("choice_%d_end", round)
		selection.TaskIndex = append(selection.TaskIndex, planningTaskReference{ID: model.LedgerItemID(stop), Goal: "No additional task is referenced."})
		if _, err := renderParserStepContext(Assignment{ParserStep: &selection}); err != nil {
			return selected, err
		}
		var choice string
		if err := s.leaf(ctx, selection, &choice); err != nil {
			return selected, err
		}
		if choice == stop {
			break
		}
		index := choices[choice] // leaf validation guarantees membership in this round.
		selected.Tasks = append(selected.Tasks, remaining[index])
		remaining = append(remaining[:index], remaining[index+1:]...)
	}
	return selected, nil
}

func validatePlanningChoice(step *ParserStep, choice string) error {
	if len(step.TaskIndex) < 2 {
		return fmt.Errorf("planning reference choice requires multiple alternatives")
	}
	for _, candidate := range step.TaskIndex {
		if string(candidate.ID) == choice {
			return nil
		}
	}
	return fmt.Errorf("planning reference resolution returned an unknown choice %q", choice)
}
