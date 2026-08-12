package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

const InitialMicrogauntletFixtureVersionV2 = "microgauntlets.v2"

type Surface string

const (
	SurfaceSymbolic   Surface = "symbolic"
	SurfaceFilesystem Surface = "filesystem"
	SurfaceRecord     Surface = "record"
)

type MicrogauntletSpec struct {
	CaseID         string                    `json:"case_id"`
	FixtureVersion string                    `json:"fixture_version"`
	Generator      labyrinth.GeneratorConfig `json:"generator"`
	Budget         RunBudget                 `json:"budget"`
}

type MicrogauntletCase struct {
	spec      MicrogauntletSpec
	generated labyrinth.GeneratedCase
}

func (spec MicrogauntletSpec) Validate() error {
	if err := requireExact(spec.CaseID, "microgauntlet case ID", 256); err != nil {
		return err
	}
	if spec.FixtureVersion != InitialMicrogauntletFixtureVersionV2 {
		return fmt.Errorf("microgauntlet fixture version is not registered")
	}
	if err := spec.Generator.Validate(); err != nil {
		return err
	}
	if err := spec.Budget.Validate(); err != nil {
		return err
	}
	if spec.Budget.EnvironmentActions < spec.Generator.Difficulty.SolutionDepth ||
		spec.Budget.ToolOperations < spec.Generator.Difficulty.SolutionDepth ||
		spec.Budget.RuntimeCycles < spec.Generator.Difficulty.SolutionDepth+1 {
		return fmt.Errorf("microgauntlet budget cannot execute its declared solution depth")
	}
	return nil
}

func (surface Surface) Version() (string, error) {
	switch surface {
	case SurfaceSymbolic:
		return "symbolic.v1", nil
	case SurfaceFilesystem:
		return labyrinth.FilesystemSurfaceVersionV1, nil
	case SurfaceRecord:
		return labyrinth.RecordSurfaceVersionV1, nil
	default:
		return "", fmt.Errorf("cognition gauntlet surface %q is not registered", surface)
	}
}

func (fixture MicrogauntletCase) Spec() MicrogauntletSpec { return fixture.spec }

func (fixture MicrogauntletCase) PublicArtifact() labyrinth.GeneratedScenario {
	return fixture.generated.PublicArtifact()
}

// SealedEnvironmentScenario is benchmark-host authority. It must remain in the
// environment/evaluator process and must never be serialized to inference.
func (fixture MicrogauntletCase) SealedEnvironmentScenario() labyrinth.Scenario {
	return fixture.generated.ExecutionScenario()
}
