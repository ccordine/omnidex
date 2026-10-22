package codingagent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

// A complete task is extracted once. Identity and lifecycle are supplied here,
// without splitting its instructions or projecting its supporting details again.
func mapPlanningResponse(call Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.Role == PlanningInitial {
		var parsed struct {
			Ledger EngineeringLedger `json:"ledger"`
		}
		if err := json.Unmarshal(content, &parsed); err != nil {
			return nil, err
		}
		parsed.Ledger.ID, parsed.Ledger.Revision = fmt.Sprintf("planning-%d", call.JobID), 1
		for index := range parsed.Ledger.Tasks {
			bindPlanningTask(&parsed.Ledger.Tasks[index], call, index+1, int64(index+1), 1)
		}
		return json.Marshal(parsed)
	}
	if call.PlanningLedger == nil {
		return nil, fmt.Errorf("planning mutation requires the current ledger")
	}
	operations, err := decodePlanningOperations(content, *call.PlanningLedger)
	if err != nil {
		return nil, err
	}
	var rank int64
	for _, task := range call.PlanningLedger.Tasks {
		rank = max(rank, task.Rank)
	}
	for index := range operations {
		op := &operations[index]
		if op.Kind == model.LedgerMutationAdd && op.Item != nil && op.Item.Task != nil {
			rank++
			bindPlanningTask(op.Item.Task, call, index+1, rank, call.PlanningLedger.Revision)
		} else if op.Kind == model.LedgerMutationAmend && op.Item != nil && op.Item.Task != nil {
			original := retainedPlanningItem(*call.PlanningLedger, op.TargetID)
			if original != nil && original.Task != nil {
				bindPlanningDetails(op.Item.Task, original.Task, model.LedgerProvenance{Kind: "final_response_statement", Reference: string(op.TargetID), Revision: call.PlanningLedger.Revision})
			}
		}
	}
	return json.Marshal(struct {
		Operations []model.LedgerMutationOperation `json:"operations"`
	}{operations})
}

func bindPlanningTask(task *LedgerTask, call Assignment, index int, rank, revision int64) {
	source := fmt.Sprintf("%x", sha256.Sum256([]byte(call.PlanningResponse)))[:12]
	id := model.LedgerItemID(fmt.Sprintf("p_%s_%d_task", source, index))
	provenance := model.LedgerProvenance{Kind: "final_response_statement", Reference: string(id), Revision: revision}
	task.LedgerItem = LedgerItem{ID: id, Provenance: provenance, Rank: rank,
		Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}
	bindPlanningDetails(task, nil, provenance)
}

func bindPlanningDetails(task, original *LedgerTask, provenance model.LedgerProvenance) {
	contexts := map[string]model.LedgerProvenance{}
	criteria := map[string]string{}
	used := map[string]bool{}
	if original != nil {
		for _, value := range original.OwnerContext {
			contexts[value.Text] = value.Provenance
		}
		for _, value := range original.Acceptance {
			criteria[value.Text], used[value.ID] = value.ID, true
		}
	}
	for index := range task.OwnerContext {
		context := &task.OwnerContext[index]
		context.Provenance = provenance
		if retained, exists := contexts[context.Text]; exists {
			context.Provenance = retained
		}
	}
	next := 1
	for index := range task.Acceptance {
		criterion := &task.Acceptance[index]
		if retained, exists := criteria[criterion.Text]; exists {
			criterion.ID = retained
			continue
		}
		for used[fmt.Sprintf("criterion_%d", next)] {
			next++
		}
		criterion.ID = fmt.Sprintf("criterion_%d", next)
		used[criterion.ID] = true
	}
}
