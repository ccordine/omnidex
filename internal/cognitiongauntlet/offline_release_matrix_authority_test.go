package cognitiongauntlet

import (
	"testing"
)

func TestReleaseMatrixCoverageRequiresEveryIsolatedScenarioSuite(t *testing.T) {
	diagnostic := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 6)
	if err := validateReleaseMatrixCoverage(diagnostic); err == nil {
		t.Fatal("one-suite diagnostic matrix was admitted as release coverage")
	}

	complete, err := NewOfflineMatrixPreregistration(OfflineMatrixPlan{
		Policy: CompetenceSuccessSuperiority, Suites: releaseMatrixSuites(),
		Seeds: []uint64{501}, Repetitions: 1, Surface: SurfaceFilesystem,
	}, matrixFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReleaseMatrixCoverage(complete); err != nil {
		t.Fatalf("complete preregistered suite coverage: %v", err)
	}

	changed := complete
	changed.Suites = append([]Suite{}, complete.Suites[:len(complete.Suites)-1]...)
	if err := validateReleaseMatrixCoverage(changed); err == nil {
		t.Fatal("release coverage accepted a removed isolated suite")
	}
}
