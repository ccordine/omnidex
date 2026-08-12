package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func GenerateMicrogauntlet(spec MicrogauntletSpec) (MicrogauntletCase, error) {
	if err := spec.Validate(); err != nil {
		return MicrogauntletCase{}, err
	}
	generated, err := labyrinth.Generate(spec.Generator)
	if err != nil {
		return MicrogauntletCase{}, fmt.Errorf("generate microgauntlet %q: %w", spec.CaseID, err)
	}
	if err := validateMicrogauntletSemantics(spec, generated); err != nil {
		return MicrogauntletCase{}, fmt.Errorf("validate microgauntlet %q: %w", spec.CaseID, err)
	}
	fixture := MicrogauntletCase{spec: spec, generated: generated}
	if _, err := fixture.PublicManifest(SurfaceSymbolic); err != nil {
		return MicrogauntletCase{}, err
	}
	if _, err := fixture.oracleManifest(); err != nil {
		return MicrogauntletCase{}, err
	}
	return fixture, nil
}

func GenerateInitialMicrogauntletsV2() ([]MicrogauntletCase, error) {
	specs := InitialMicrogauntletsV2()
	fixtures := make([]MicrogauntletCase, len(specs))
	seenSuites := make(map[Suite]struct{}, len(specs))
	for index, spec := range specs {
		fixture, err := GenerateMicrogauntlet(spec)
		if err != nil {
			return nil, err
		}
		suite, err := gauntletSuite(spec.Generator.Suite)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seenSuites[suite]; duplicate {
			return nil, fmt.Errorf("initial microgauntlet suite %q is duplicated", suite)
		}
		seenSuites[suite] = struct{}{}
		fixtures[index] = fixture
	}
	if len(seenSuites) != 5 {
		return nil, fmt.Errorf("initial microgauntlets require exactly five isolated suites")
	}
	return fixtures, nil
}
