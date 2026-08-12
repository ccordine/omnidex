package cognitiongauntlet

import (
	"testing"
)

func TestReleaseMatrixCoverageRequiresEveryIsolatedScenarioSuite(t *testing.T) {
	diagnostic := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 6)
	qualified, diagnosticSHA, err := deriveReleaseMatrixCoverage(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if qualified || !validDigest(diagnosticSHA) {
		t.Fatal("one-suite diagnostic matrix was admitted as release coverage")
	}
	suites, err := registeredReleaseMatrixSuites()
	if err != nil {
		t.Fatal(err)
	}

	complete, err := NewOfflineMatrixPreregistration(OfflineMatrixPlan{
		Policy: CompetenceSuccessSuperiority, Suites: suites,
		Seeds: []uint64{501}, Repetitions: 1, Surface: SurfaceFilesystem,
	}, matrixFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	qualified, completeSHA, err := deriveReleaseMatrixCoverage(complete)
	if err != nil || !qualified || !validDigest(completeSHA) {
		t.Fatalf("complete preregistered suite coverage=%t hash=%q error=%v", qualified, completeSHA, err)
	}

	changed := complete
	changed.Suites = append([]Suite{}, complete.Suites[:len(complete.Suites)-1]...)
	if _, _, err := deriveReleaseMatrixCoverage(changed); err == nil {
		t.Fatal("release coverage accepted an altered preregistration authority")
	}
}

func TestReleaseMatrixSuitesAreDerivedFromExecutableScenarioRegistries(t *testing.T) {
	suites, err := registeredReleaseMatrixSuites()
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[Suite]struct{}, len(suites))
	for _, suite := range suites {
		registered[suite] = struct{}{}
	}
	for _, spec := range InitialMicrogauntletsV2() {
		suite, err := gauntletSuite(spec.Generator.Suite)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := registered[suite]; !ok {
			t.Fatalf("registered initial suite %q is absent from release coverage", suite)
		}
	}
	for _, definition := range ExtendedSuitesV1() {
		_, included := registered[definition.Suite]
		want := definition.Execution == ExecutionScenario && definition.Executable
		if included != want {
			t.Fatalf(
				"registered extended suite %q inclusion=%t want=%t",
				definition.Suite, included, want,
			)
		}
	}
}
