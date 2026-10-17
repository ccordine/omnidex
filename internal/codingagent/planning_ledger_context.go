package codingagent

import "github.com/gryph/omnidex/internal/model"

type PlanningLedgerContext struct {
	Revision int64                 `json:"revision"`
	Items    []PlanningContextItem `json:"items"`
}
type PlanningContextItem struct {
	ID          model.LedgerItemID      `json:"id"`
	Kind        string                  `json:"kind"`
	Text        string                  `json:"text"`
	ReviewState model.LedgerReviewState `json:"review_state,omitempty"`
	ReadFiles   []string                `json:"read_files,omitempty"`
	WriteFiles  []string                `json:"write_files,omitempty"`
	DeleteFiles []string                `json:"delete_files,omitempty"`
}

func ActivePlanningLedgerContext(ledger *EngineeringLedger, completedTaskIDs ...string) *PlanningLedgerContext {
	if ledger == nil {
		return nil
	}
	out := &PlanningLedgerContext{Revision: ledger.Revision, Items: []PlanningContextItem{}}
	for _, task := range ledger.Tasks {
		if task.Disposition != model.LedgerDispositionActive || contains(completedTaskIDs, string(task.ID)) {
			continue
		}
		out.Items = append(out.Items, PlanningContextItem{ID: task.ID, Kind: "task", Text: task.Goal, ReviewState: task.ReviewState, ReadFiles: task.ReadFiles, WriteFiles: task.WriteFiles, DeleteFiles: task.DeleteFiles})
	}
	return out
}
