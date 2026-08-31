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
	assembly, err := driver.Assemble()
	if err != nil {
		return failDirectCodingWorkflow(driver, "compile deterministic assembly", err)
	}
	driver.Phase(directCodingPhaseConstructing, fmt.Sprintf("files=%d deletes=%d", len(assembly.Files), len(assembly.DeletePaths)))
	prepared, err := driver.PrepareAssembly(assembly)
	if err != nil {
		return failDirectCodingWorkflow(driver, "prepare exact workspace mutation", err)
	}
	if prepared == nil {
		return failDirectCodingWorkflow(driver, "prepare exact workspace mutation", fmt.Errorf("driver returned no prepared mutation"))
	}
	driver.Phase(directCodingPhaseVerifying, "verifying exact workspace post-state")
	if err := driver.ApplyAndVerify(prepared); err != nil {
		return failDirectCodingWorkflow(driver, "verify accepted workspace", err)
	}
	summary := driver.Complete()
	driver.Phase(directCodingPhaseCompleted, summary)
	return summary, nil
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
