package worker

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingPhase string

const (
	directCodingPhaseAssembling   directCodingPhase = "assembling"
	directCodingPhaseConstructing directCodingPhase = "constructing"
	directCodingPhaseVerifying    directCodingPhase = "verifying"
	directCodingPhaseDeploying    directCodingPhase = "deploying"
	directCodingPhaseCompleted    directCodingPhase = "completed"
	directCodingPhaseFailed       directCodingPhase = "failed"
)

type directCodingDiagnostic struct {
	Stage      string
	Command    string
	TargetPath string
	Detail     string
}

func (d directCodingDiagnostic) validate() error {
	if strings.TrimSpace(d.Stage) == "" {
		return fmt.Errorf("coding diagnostic requires a stage")
	}
	if strings.TrimSpace(d.Detail) == "" {
		return fmt.Errorf("coding diagnostic requires exact failure detail")
	}
	target := strings.TrimSpace(d.TargetPath)
	if target == "" {
		return nil
	}
	normalizedTarget, err := normalizeDirectCodingPath(target)
	if err != nil {
		return fmt.Errorf("coding diagnostic target path: %w", err)
	}
	if target != normalizedTarget {
		return fmt.Errorf("coding diagnostic target path %q must be normalized as %q", target, normalizedTarget)
	}
	return nil
}

func directCodingStaticFileDiagnostic(path, detail string, _ ...string) *directCodingDiagnostic {
	return &directCodingDiagnostic{
		Stage: "static_validation", TargetPath: path, Detail: detail,
	}
}

type directCodingVerification struct {
	Passed      bool
	TestsPassed bool
	Commands    []string
	EvidenceIDs []int64
	Diagnostic  *directCodingDiagnostic
}

func (v directCodingVerification) validate() error {
	if len(v.Commands) != len(v.EvidenceIDs) ||
		len(v.Commands) > queue.MaxGeneratedWorkloadVerificationEvidence-1 {
		return fmt.Errorf("coding verification command and evidence identities must be exact")
	}
	for index, id := range v.EvidenceIDs {
		if id <= 0 || index > 0 && id <= v.EvidenceIDs[index-1] {
			return fmt.Errorf("coding verification evidence identities must be ordered")
		}
	}
	if v.Passed {
		if v.Diagnostic != nil {
			return fmt.Errorf("successful coding verification cannot include a diagnostic")
		}
		if len(v.Commands) == 0 {
			return fmt.Errorf("successful coding verification requires at least one executed command")
		}
		return nil
	}
	if v.Diagnostic == nil {
		return fmt.Errorf("failed coding verification requires an exact diagnostic")
	}
	return v.Diagnostic.validate()
}

type directCodingWorkflowDriver interface {
	Phase(phase directCodingPhase, detail string)
	Assemble() (directCodingAssembly, error)
	EnsureDirectory(path string) (bool, error)
	Delete(path string) (bool, error)
	MaterializeTask(task directCodingFileTask) (bool, error)
	BeginVerification() (directCodingCompletionTaskDisposition, error)
	Verify() (directCodingVerification, error)
	FinalizeVerified(
		verification directCodingVerification,
		beginState directCodingCompletionTaskDisposition,
	) error
	Complete(verification directCodingVerification) (string, error)
}

func runDirectCodingWorkflow(driver directCodingWorkflowDriver, allowExistingWorkspace bool) (string, error) {
	if driver == nil {
		return "", fmt.Errorf("direct coding workflow requires a driver")
	}
	driver.Phase(directCodingPhaseAssembling, "compiling deterministic source assembly")
	assembly, err := driver.Assemble()
	if err != nil {
		return failDirectCodingWorkflow(driver, "compile deterministic assembly", err)
	}
	if err := assembly.validate(); err != nil {
		return failDirectCodingWorkflow(driver, "validate deterministic assembly", err)
	}

	driver.Phase(directCodingPhaseConstructing, fmt.Sprintf("directories=%d files=%d deletes=%d", len(assembly.Directories), len(assembly.Files), len(assembly.DeletePaths)))
	mutations := 0
	for _, path := range assembly.Directories {
		changed, ensureErr := driver.EnsureDirectory(path)
		if ensureErr != nil {
			return failDirectCodingWorkflow(driver, "ensure directory "+path, ensureErr)
		}
		if changed {
			mutations++
		}
	}
	for _, path := range assembly.DeletePaths {
		changed, deleteErr := driver.Delete(path)
		if deleteErr != nil {
			return failDirectCodingWorkflow(driver, "delete "+path, deleteErr)
		}
		if changed {
			mutations++
		}
	}
	for _, task := range assembly.Files {
		changed, generateErr := driver.MaterializeTask(task)
		if generateErr != nil {
			return failDirectCodingWorkflow(driver, "generate "+task.Path, generateErr)
		}
		if changed {
			mutations++
		}
	}
	if mutations == 0 && !allowExistingWorkspace {
		return failDirectCodingWorkflow(driver, "generate coding files", fmt.Errorf("fresh coding workflow accepted no workspace mutation"))
	}

	driver.Phase(directCodingPhaseVerifying, "running code-selected verification")
	verificationBeginState, err := driver.BeginVerification()
	if err != nil {
		return failDirectCodingWorkflow(driver, "begin workspace verification", err)
	}
	verification, verifyErr := driver.Verify()
	if verifyErr != nil {
		return failDirectCodingWorkflow(driver, "verify accepted workspace", verifyErr)
	}
	if err := verification.validate(); err != nil {
		return failDirectCodingWorkflow(driver, "validate verification result", err)
	}
	if !verification.Passed {
		return failDirectCodingWorkflow(driver, "verify completed program", fmt.Errorf(
			"%s: %s",
			safeLine(verification.Diagnostic.Stage, "unknown"),
			trimForBudget(verification.Diagnostic.Detail, 1200),
		))
	}
	if err := driver.FinalizeVerified(verification, verificationBeginState); err != nil {
		return failDirectCodingWorkflow(driver, "finalize verified workspace", err)
	}
	summary, completeErr := driver.Complete(verification)
	if completeErr != nil {
		return failDirectCodingWorkflow(driver, "complete coding workflow", completeErr)
	}
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
