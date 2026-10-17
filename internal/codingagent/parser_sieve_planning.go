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
	Operations            []planningParserOperation `json:"operations"`
	ExistingConstraintIDs []model.LedgerItemID      `json:"existing_constraint_ids,omitempty"`
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
	global := []model.LedgerItemID{}
	unit := 0
	sourceHash := fmt.Sprintf("%x", sha256.Sum256([]byte(s.call.PlanningResponse)))[:12]
	for index, parts := range s.parts {
		for _, part := range parts {
			if part.Kind == "ignore" {
				continue
			}
			unit++
			if part.Kind == "change" {
				if id := representedPlanningStatement(part.Text, current, operations); id != "" {
					if err := s.receipt("planning_restatement", map[string]any{
						"block_id": s.blocks[index].ID, "text": part.Text, "item_id": id,
					}); err != nil {
						return nil, err
					}
					continue
				}
			}
			step := s.step(index, "planning_unit")
			step.Source, step.Category, step.Ledger = part.Text, part.Kind, &current
			step.NewIDs = map[string]string{}
			for _, kind := range planningUnitItemKinds("change") {
				step.NewIDs[kind] = fmt.Sprintf("p_%s_%d_%s", sourceHash, unit, kind)
			}
			var raw json.RawMessage
			if err := s.leaf(ctx, step, &raw); err != nil {
				return nil, err
			}
			var projection parserPlanningProjection
			if err := decodeContract(raw, &projection); err != nil {
				return nil, err
			}
			for _, id := range projection.ExistingConstraintIDs {
				found := false
				for _, c := range current.Constraints {
					found = found || c.ID == id && c.Disposition == model.LedgerDispositionActive
				}
				if !found {
					return nil, fmt.Errorf("source policy references an unknown or inactive constraint")
				}
				global = append(global, id)
			}
			raw, _ = json.Marshal(struct {
				Operations []planningParserOperation `json:"operations"`
			}{projection.Operations})
			normalized, err := normalizeParserPlanningItems(raw, part, step, current)
			if err != nil {
				return nil, fmt.Errorf("%s planning statement: %w", step.BlockID, err)
			}
			parsed, err := decodePlanningOperations(normalized, current)
			if err != nil {
				return nil, err
			}
			if len(parsed) == 0 && len(projection.ExistingConstraintIDs) == 0 {
				return nil, fmt.Errorf("admitted planning statement produced no ledger effect")
			}
			for _, operation := range parsed {
				if err := applyPlanningOperation(&current, operation); err != nil {
					return nil, err
				}
				if part.Kind == "project_constraint" && operation.Item != nil && operation.Item.Constraint != nil {
					global = append(global, operation.Item.Constraint.ID)
				}
			}
			if err := current.ValidateDraft(); err != nil {
				return nil, err
			}
			operations = append(operations, parsed...)
		}
	}
	// Project-wide policy has a deterministic consumer: every remaining task,
	// including tasks described later in the same response. No model reauthors it.
	for _, task := range append([]model.LedgerTask(nil), current.Tasks...) {
		if task.Disposition != model.LedgerDispositionActive || contains(s.call.CompletedTaskIDs, string(task.ID)) {
			continue
		}
		ids := append([]model.LedgerItemID{}, task.ConstraintIDs...)
		for _, id := range global {
			found := false
			for _, old := range ids {
				found = found || old == id
			}
			if !found {
				ids = append(ids, id)
			}
		}
		if len(ids) == len(task.ConstraintIDs) {
			continue
		}
		patch, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{
			"kind": "amend", "target_id": task.ID, "item": map[string]any{"task": map[string]any{"constraint_ids": ids}}, "rationale": "Bind the stated project-wide constraint to its remaining task consumer.",
		}}})
		bound, err := decodePlanningOperations(patch, current)
		if err != nil {
			return nil, err
		}
		if err := applyPlanningOperation(&current, bound[0]); err != nil {
			return nil, err
		}
		operations = append(operations, bound...)
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
		if base.Priority == "" {
			base.Priority = model.LedgerPriorityNormal
		}
		// Every added obligation retains its entire admitted source span. The
		// projection supplies relationships, never a lossy replacement summary.
		switch {
		case item.Task != nil:
			item.Task.Goal = strings.TrimSpace(part.Text)
			for _, list := range [][]string{item.Task.ReadFiles, item.Task.WriteFiles, item.Task.DeleteFiles} {
				for _, file := range list {
					if !strings.Contains(part.Text, file) {
						return nil, fmt.Errorf("parser invented a file absent from its source statement: %s", file)
					}
				}
			}
		case item.Requirement != nil:
			item.Requirement.Outcome = strings.TrimSpace(part.Text)
		case item.AcceptanceCriterion != nil:
			item.AcceptanceCriterion.Outcome = strings.TrimSpace(part.Text)
		case item.Constraint != nil:
			item.Constraint.Rule = strings.TrimSpace(part.Text)
		case item.Objective != nil:
			item.Objective.Outcome = strings.TrimSpace(part.Text)
		case item.Check != nil:
			if len(item.Check.Argv) == 0 {
				item.Check.Observation = strings.TrimSpace(part.Text)
			}
			for _, arg := range item.Check.Argv {
				if arg == "" || !strings.Contains(part.Text, arg) {
					return nil, fmt.Errorf("parser invented an unstated verification command argument")
				}
			}
		}
		op.Item, _ = json.Marshal(item)
	}
	return json.Marshal(parsed)
}
