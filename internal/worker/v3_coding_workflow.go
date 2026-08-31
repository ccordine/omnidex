package worker

import (
	"fmt"
	"strings"
	"unicode"
)

type directCodingPhase string

const (
	directCodingPhaseAssembling   directCodingPhase = "assembling"
	directCodingPhaseConstructing directCodingPhase = "constructing"
	directCodingPhaseVerifying    directCodingPhase = "verifying"
	directCodingPhaseCompleted    directCodingPhase = "completed"
	directCodingPhaseFailed       directCodingPhase = "failed"
)

type directCodingWorkflowDriver interface {
	Phase(phase directCodingPhase, detail string)
	Assemble() (directCodingAssembly, error)
	PrepareAssembly(directCodingAssembly) (*directCodingPreparedMutation, error)
	ApplyAndVerify(*directCodingPreparedMutation) error
	Complete() string
}

func runDirectCodingWorkflow(driver directCodingWorkflowDriver) (string, error) {
	if driver == nil {
		return "", fmt.Errorf("direct coding workflow requires a driver")
	}
	driver.Phase(directCodingPhaseAssembling, "compiling deterministic source assembly")
	assembly, assemblyErr := driver.Assemble()
	if assemblyErr != nil {
		return failDirectCodingWorkflow(driver, "compile deterministic assembly", assemblyErr)
	}
	if !directCodingAssemblyHasDesiredState(assembly) {
		return failDirectCodingWorkflow(
			driver,
			"compile deterministic assembly",
			fmt.Errorf(
				"NO_EXECUTABLE_WORK_DERIVED: actionable coding objective produced no accepted desired state",
			),
		)
	}
	if err := applyDirectCodingAssembly(driver, assembly); err != nil {
		return failDirectCodingWorkflow(driver, "apply deterministic assembly", err)
	}
	summary := driver.Complete()
	driver.Phase(directCodingPhaseCompleted, summary)
	return summary, nil
}

// directCodingAssemblyHasDesiredState reports accepted filesystem obligations,
// not observed filesystem changes. Reconciliation may prove that this desired
// state already exists and correctly return a zero-delta success.
func directCodingAssemblyHasDesiredState(assembly directCodingAssembly) bool {
	return len(assembly.Files) > 0 || len(assembly.RequiredPaths) > 0 || len(assembly.DeletePaths) > 0
}

func applyDirectCodingAssembly(
	driver directCodingWorkflowDriver,
	assembly directCodingAssembly,
) error {
	driver.Phase(directCodingPhaseConstructing, fmt.Sprintf("files=%d deletes=%d", len(assembly.Files), len(assembly.DeletePaths)))
	prepared, err := driver.PrepareAssembly(assembly)
	if err != nil {
		return fmt.Errorf("prepare exact workspace mutation: %w", err)
	}
	if prepared == nil {
		return fmt.Errorf("prepare exact workspace mutation: driver returned no prepared mutation")
	}
	driver.Phase(directCodingPhaseVerifying, "verifying exact workspace post-state")
	if err := driver.ApplyAndVerify(prepared); err != nil {
		return fmt.Errorf("verify accepted workspace: %w", err)
	}
	return nil
}

func directCodingEventToken(value, fallback string) string {
	value = safeLine(value, fallback)
	return strings.Map(func(char rune) rune {
		if unicode.IsSpace(char) {
			return '_'
		}
		return char
	}, value)
}

func failDirectCodingWorkflow(driver directCodingWorkflowDriver, stage string, err error) (string, error) {
	detail := fmt.Sprintf("%s: %v", stage, err)
	driver.Phase(directCodingPhaseFailed, detail)
	return "", fmt.Errorf("%s: %w", stage, err)
}
