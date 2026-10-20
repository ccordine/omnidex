package queue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestReplanTailRetainsRecoveredStepHistory(t *testing.T) {
	boundary, err := canonicalReplanTail([]stepSeed{{action: "v3_coding_plan", sortIndex: 5}, {action: "v3_coding", sortIndex: 10}})
	if err != nil {
		t.Fatal(err)
	}
	rows := []replanStepRecord{
		{ID: 164, Action: "v3_coding_plan", SortIndex: 5, Status: model.StepStatusCompleted, Generation: 6},
		{ID: 165, Action: "v3_coding", SortIndex: 10, Status: model.StepStatusFailed, Generation: 6},
		{ID: 166, Action: "v3_coding", SortIndex: 10, Status: model.StepStatusWaiting, Generation: 6},
	}
	ids, err := validateCurrentReplanTail(6, boundary, rows)
	if err != nil || !reflect.DeepEqual(ids, []int64{164, 165, 166}) {
		t.Fatalf("recovered canonical tail lost retirement history: %v, %v", ids, err)
	}
	rows[2].Status = model.StepStatusFailed
	rows = append(rows, replanStepRecord{ID: 167, Action: "v3_coding", SortIndex: 10, Status: model.StepStatusPending, Generation: 6})
	ids, err = validateCurrentReplanTail(6, boundary, rows)
	if err != nil || !reflect.DeepEqual(ids, []int64{164, 165, 166, 167}) {
		t.Fatalf("repeated recovery lost retirement history: %v, %v", ids, err)
	}
	for name, mutate := range map[string]func([]replanStepRecord){
		"two active steps":      func(r []replanStepRecord) { r[1].Status = model.StepStatusRunning },
		"completed predecessor": func(r []replanStepRecord) { r[1].Status = model.StepStatusCompleted },
		"canceled predecessor":  func(r []replanStepRecord) { r[1].Status = model.StepStatusCanceled },
		"different action":      func(r []replanStepRecord) { r[2].Action = "other" },
		"reversed history":      func(r []replanStepRecord) { r[1].ID, r[2].ID = r[2].ID, r[1].ID },
		"duplicate identity":    func(r []replanStepRecord) { r[2].ID = r[1].ID },
		"stale generation":      func(r []replanStepRecord) { r[2].Generation = 5 },
		"unregistered status":   func(r []replanStepRecord) { r[2].Status = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := append([]replanStepRecord(nil), rows[:3]...)
			mutate(invalid)
			if _, err := validateCurrentReplanTail(6, boundary, invalid); !errors.Is(err, ErrInvalidJobGeneration) {
				t.Fatalf("invalid duplicate position accepted: %v", err)
			}
		})
	}
}
