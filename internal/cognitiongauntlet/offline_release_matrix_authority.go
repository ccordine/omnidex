package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"sort"
)

const releaseMatrixCoverageSchemaV1 = "omnidex.offline-release-matrix-coverage.v1"

// deriveReleaseMatrixCoverage proves only coordinate completeness. Sample size
// and repetitions remain the exact values sealed by the preregistration; this
// function does not invent a minimum absent from the Labyrinth contract.
func deriveReleaseMatrixCoverage(
	registration OfflineMatrixPreregistration,
) (bool, string, error) {
	if err := registration.Validate(); err != nil {
		return false, "", err
	}
	expected, err := registeredReleaseMatrixSuites()
	if err != nil {
		return false, "", err
	}
	qualified := reflect.DeepEqual(registration.Suites, expected) &&
		registration.SampleCount ==
			len(registration.Suites)*len(registration.Seeds)*registration.Repetitions
	digest, err := digestJSON(struct {
		Schema         string  `json:"schema"`
		ExpectedSuites []Suite `json:"expected_suites"`
		ActualSuites   []Suite `json:"actual_suites"`
		SampleCount    int     `json:"sample_count"`
		Qualified      bool    `json:"qualified"`
	}{
		Schema: releaseMatrixCoverageSchemaV1, ExpectedSuites: expected,
		ActualSuites: cloneMatrixSlice(registration.Suites),
		SampleCount:  registration.SampleCount, Qualified: qualified,
	})
	if err != nil {
		return false, "", err
	}
	return qualified, digest, nil
}

func registeredReleaseMatrixSuites() ([]Suite, error) {
	seen := make(map[Suite]struct{})
	suites := make([]Suite, 0)
	add := func(suite Suite) error {
		if offlineScenarioSuiteRank(suite) == 0 {
			return fmt.Errorf("registered release suite %q lacks an ordinary scenario rank", suite)
		}
		if _, duplicate := seen[suite]; duplicate {
			return fmt.Errorf("registered release suite %q is duplicated", suite)
		}
		seen[suite] = struct{}{}
		suites = append(suites, suite)
		return nil
	}
	for _, spec := range InitialMicrogauntletsV2() {
		if err := spec.Validate(); err != nil {
			return nil, fmt.Errorf("registered initial release suite: %w", err)
		}
		suite, err := gauntletSuite(spec.Generator.Suite)
		if err != nil {
			return nil, err
		}
		if err := add(suite); err != nil {
			return nil, err
		}
	}
	for _, definition := range ExtendedSuitesV1() {
		if err := definition.Validate(); err != nil {
			return nil, fmt.Errorf("registered extended release suite: %w", err)
		}
		if definition.Execution != ExecutionScenario {
			continue
		}
		if !definition.Executable {
			return nil, fmt.Errorf(
				"registered scenario suite %q is not executable", definition.Suite,
			)
		}
		if err := add(definition.Suite); err != nil {
			return nil, err
		}
	}
	sort.Slice(suites, func(left, right int) bool {
		return offlineScenarioSuiteRank(suites[left]) < offlineScenarioSuiteRank(suites[right])
	})
	return suites, nil
}
