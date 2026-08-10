package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

type OfflineMatrixOracleBound struct {
	Case                  OfflineMatrixCase `json:"case"`
	OracleSHA256          string            `json:"oracle_sha256"`
	Quality               OracleQuality     `json:"quality"`
	ReferenceDecisionCost int64             `json:"reference_decision_cost"`
	TaskArchetype         string            `json:"task_archetype"`
}

func deriveOfflineMatrixOracleBounds(
	registration OfflineMatrixPreregistration,
	runs []OfflineMatrixRunReceipt,
) ([]OfflineMatrixOracleBound, error) {
	indexed, err := indexOfflineMatrixRuns(registration, runs)
	if err != nil {
		return nil, err
	}
	bounds := make([]OfflineMatrixOracleBound, 0, len(registration.Cases))
	for _, currentCase := range registration.Cases {
		var bound OfflineMatrixOracleBound
		for variantIndex, variant := range registration.Variants {
			run, exists := indexed[currentCase.ID+"\x00"+string(variant)]
			if !exists {
				return nil, fmt.Errorf("offline matrix oracle bound lacks variant %q", variant)
			}
			candidate := OfflineMatrixOracleBound{
				Case: currentCase, OracleSHA256: run.OracleSHA256,
				Quality: run.OracleQuality, ReferenceDecisionCost: run.OracleReferenceDecisionCost,
				TaskArchetype: run.TaskArchetype,
			}
			if variantIndex == 0 {
				bound = candidate
				continue
			}
			if !reflect.DeepEqual(bound, candidate) {
				return nil, fmt.Errorf("offline matrix variants disagree on private oracle bound for %q", currentCase.ID)
			}
		}
		bounds = append(bounds, bound)
	}
	return bounds, nil
}

func equalOfflineMatrixOracleBounds(
	left []OfflineMatrixOracleBound,
	right []OfflineMatrixOracleBound,
) bool {
	return reflect.DeepEqual(left, right)
}
