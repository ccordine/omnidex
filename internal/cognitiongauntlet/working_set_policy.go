package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/workingset"
)

const (
	fullCognitionWorkingSetItems = 32
)

func fullCognitionWorkingSetBudget(maxBytes int) (workingset.Budget, error) {
	if maxBytes <= 0 || maxBytes > workingset.MaxResidentBytes {
		return workingset.Budget{}, fmt.Errorf("full cognition Working Set byte budget is invalid")
	}
	return workingset.Budget{
		MaxItems: fullCognitionWorkingSetItems, MaxBytes: maxBytes,
		MaxPinnedItems: fullCognitionWorkingSetItems, MaxPinnedBytes: maxBytes,
	}, nil
}
