package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

// validateReleaseMatrixCoverage proves only coordinate completeness. Sample
// size and repetitions remain the exact values sealed by the preregistration;
// this function does not invent a minimum absent from the Labyrinth contract.
func validateReleaseMatrixCoverage(registration OfflineMatrixPreregistration) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	expected := releaseMatrixSuites()
	if !reflect.DeepEqual(registration.Suites, expected) {
		return fmt.Errorf("release matrix omits one or more isolated scenario suites")
	}
	if registration.SampleCount !=
		len(registration.Suites)*len(registration.Seeds)*registration.Repetitions {
		return fmt.Errorf("release matrix sample authority is inconsistent")
	}
	return nil
}

func releaseMatrixSuites() []Suite {
	return []Suite{
		SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined,
		SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder,
	}
}
