package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) Phase(phase directCodingPhase, detail string) {
	detail = trimForBudget(strings.TrimSpace(detail), 1200)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_phase_changed", fmt.Sprintf(
		"phase=%s detail=%s",
		phase,
		safeLine(detail, "none"),
	))
}

func (s *directCodingSession) Assemble() (directCodingAssembly, error) {
	authorityParts := []string{s.request.Instruction}
	authorityParts = append(authorityParts, s.request.AdditionalAuthority...)
	authorityParts = append(authorityParts, s.request.Feedback...)
	authority := strings.TrimSpace(strings.Join(authorityParts, "\n"))
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority)
	if err != nil {
		return directCodingAssembly{}, err
	}
	surfaceModel, err := s.workerModel(station.CodingSurface)
	if err != nil {
		return directCodingAssembly{}, err
	}
	requirementModel, err := s.workerModel(station.CodingRequirements)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workloadModel, err := s.workerModel(station.CodingWorkload)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workloadReviewModel, err := s.workerModel(station.CodingWorkloadReview)
	if err != nil {
		return directCodingAssembly{}, err
	}
	artifactModel, err := s.workerModel(station.CodingArtifactHandling)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	specification, err := runDirectCodingApplicationInterpreter(
		workerRuntime, requirementModel, surfaceModel, artifactModel, redacted, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := validateDirectCodingRequirementCount(specification.Requirements); err != nil {
		return directCodingAssembly{}, err
	}
	workloadInput := applicationWorkloadInput(specification)
	workload, err := resolveDirectCodingApplicationWorkload(
		workerRuntime, workloadModel, workloadReviewModel, workloadInput,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_workload_frozen", fmt.Sprintf(
		"tasks=%d sha256=%s", len(workload.Tasks), workload.SHA256,
	))
	skills, err := s.bindRequirementSkills(specification.ProductQuote, specification.Requirements)
	if err != nil {
		return directCodingAssembly{}, err
	}
	capabilities, err := s.deriveRequirementCapabilities(
		specification.ProductQuote, specification.Requirements,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	program, err := compileDirectCodingProgram(
		filepath.Base(s.root), specification, identities, skills, workload, capabilities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := s.runDirectCodingApplicationTaskLifecycle(workloadInput, workload, &program); err != nil {
		return directCodingAssembly{}, err
	}
	protectedPaths, err := snapshotDirectCodingProtectedPathList(s.root, program.ProtectedPaths)
	if err != nil {
		return directCodingAssembly{}, err
	}
	s.specification = &specification
	s.program = &program
	s.protectedPaths = protectedPaths
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := validateDirectCodingAssemblyProtection(assembly, s.protectedPaths); err != nil {
		return directCodingAssembly{}, err
	}
	for _, task := range assembly.Files {
		if _, err := resolveV3WorkspaceFile(s.root, task.Path); err != nil {
			return directCodingAssembly{}, err
		}
	}
	s.plannedFiles = len(assembly.Files)
	s.plannedDeletes = 0
	blockCount, waveCount, err := directCodingProgramGraphMetrics(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_specification_accepted", fmt.Sprintf(
		"surface=%s requirements=%d product_bytes=%d",
		specification.Surface, len(specification.Requirements), len(specification.ProductQuote),
	))
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_assembly_ready", fmt.Sprintf(
		"adapter=%s files=%d blocks=%d waves=%d",
		program.Adapter, len(assembly.Files), blockCount, waveCount,
	))
	return assembly, nil
}

func requireDirectCodingModel(id station.ID, modelName string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("direct coding station %s model is not configured", id)
	}
	return modelName, nil
}
