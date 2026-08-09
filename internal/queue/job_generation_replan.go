package queue

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

const (
	jobGenerationPurposeInitial = "initial"
	jobGenerationPurposeReplan  = "replan"
	replanCodingBoundary        = "v3_coding"
	replanPlanningBoundary      = "v3_planning"
	delegatedSubtaskAction      = "v3_subtask"
)

var ErrInvalidJobGeneration = errors.New("invalid job generation state")

type replanBoundary struct {
	action    string
	sortIndex int
	seeds     []stepSeed
}

type replanStepRecord struct {
	ID         int64
	Action     string
	SortIndex  int
	Status     string
	Generation int64
}

func canonicalReplanTail(seeds []stepSeed) (replanBoundary, error) {
	if len(seeds) == 0 {
		return replanBoundary{}, fmt.Errorf("%w: canonical job has no steps", ErrInvalidJobGeneration)
	}
	codingIndex, planningIndex := -1, -1
	previousSort := -1
	for index, seed := range seeds {
		if seed.action == "" || seed.sortIndex <= previousSort {
			return replanBoundary{}, fmt.Errorf("%w: canonical steps are not strictly ordered", ErrInvalidJobGeneration)
		}
		previousSort = seed.sortIndex
		switch seed.action {
		case replanCodingBoundary:
			if codingIndex >= 0 {
				return replanBoundary{}, fmt.Errorf("%w: duplicate %s boundary", ErrInvalidJobGeneration, replanCodingBoundary)
			}
			codingIndex = index
		case replanPlanningBoundary:
			if planningIndex >= 0 {
				return replanBoundary{}, fmt.Errorf("%w: duplicate %s boundary", ErrInvalidJobGeneration, replanPlanningBoundary)
			}
			planningIndex = index
		}
	}
	if codingIndex >= 0 && planningIndex >= 0 {
		return replanBoundary{}, fmt.Errorf("%w: canonical job has competing replan boundaries", ErrInvalidJobGeneration)
	}
	boundaryIndex := codingIndex
	if boundaryIndex < 0 {
		boundaryIndex = planningIndex
	}
	if boundaryIndex < 0 {
		return replanBoundary{}, fmt.Errorf(
			"%w: canonical job has neither %s nor %s boundary",
			ErrInvalidJobGeneration, replanCodingBoundary, replanPlanningBoundary,
		)
	}
	tail := append([]stepSeed(nil), seeds[boundaryIndex:]...)
	return replanBoundary{
		action:    seeds[boundaryIndex].action,
		sortIndex: seeds[boundaryIndex].sortIndex,
		seeds:     tail,
	}, nil
}

func validateCurrentReplanTail(
	currentGeneration int64,
	boundary replanBoundary,
	rows []replanStepRecord,
) ([]int64, error) {
	if currentGeneration <= 0 || len(boundary.seeds) == 0 || len(rows) == 0 {
		return nil, fmt.Errorf("%w: current replan tail is empty", ErrInvalidJobGeneration)
	}
	if rows[0].Action != boundary.action || rows[0].SortIndex != boundary.sortIndex {
		return nil, fmt.Errorf(
			"%w: current boundary is %s@%d, expected %s@%d",
			ErrInvalidJobGeneration,
			rows[0].Action, rows[0].SortIndex, boundary.action, boundary.sortIndex,
		)
	}
	retiringIDs := make([]int64, 0, len(rows))
	canonicalIndex := 0
	seenIDs := make(map[int64]struct{}, len(rows))
	previousSort := -1
	for _, row := range rows {
		if row.ID <= 0 {
			return nil, fmt.Errorf("%w: retiring step has invalid identity", ErrInvalidJobGeneration)
		}
		if _, exists := seenIDs[row.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate retiring step %d", ErrInvalidJobGeneration, row.ID)
		}
		seenIDs[row.ID] = struct{}{}
		if row.Generation != currentGeneration {
			return nil, fmt.Errorf(
				"%w: unsuperseded step %d belongs to generation %d, current generation is %d",
				ErrInvalidJobGeneration, row.ID, row.Generation, currentGeneration,
			)
		}
		if row.SortIndex <= previousSort {
			return nil, fmt.Errorf("%w: current replan tail is not strictly ordered", ErrInvalidJobGeneration)
		}
		previousSort = row.SortIndex
		if !registeredStepStatus(row.Status) {
			return nil, fmt.Errorf("%w: step %d has unregistered status %q", ErrInvalidJobGeneration, row.ID, row.Status)
		}
		retiringIDs = append(retiringIDs, row.ID)
		if row.Action == delegatedSubtaskAction {
			continue
		}
		if canonicalIndex >= len(boundary.seeds) || row.Action != boundary.seeds[canonicalIndex].action {
			return nil, fmt.Errorf(
				"%w: step %d action %q is not canonical tail position %d",
				ErrInvalidJobGeneration, row.ID, row.Action, canonicalIndex,
			)
		}
		canonicalIndex++
	}
	if canonicalIndex != len(boundary.seeds) {
		return nil, fmt.Errorf(
			"%w: current tail has %d canonical steps, expected %d",
			ErrInvalidJobGeneration, canonicalIndex, len(boundary.seeds),
		)
	}
	return retiringIDs, nil
}

func registeredStepStatus(status string) bool {
	switch status {
	case model.StepStatusPending, model.StepStatusRunning, model.StepStatusCompleted,
		model.StepStatusFailed, model.StepStatusWaiting, model.StepStatusCanceled:
		return true
	default:
		return false
	}
}
