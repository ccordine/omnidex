package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialist"
)

func (s *directCodingSession) Phase(phase directCodingPhase, detail string) {
	detail = trimForBudget(strings.TrimSpace(detail), 1200)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_phase_changed", fmt.Sprintf(
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
	surfaceModel, err := s.workerModel("coding_surface", specialist.RoleCodingSurfaceStation)
	if err != nil {
		return directCodingAssembly{}, err
	}
	partitionModel, err := s.workerModel("coding_requirement_partition", specialist.RoleCodingRequirementPartitionStation)
	if err != nil {
		return directCodingAssembly{}, err
	}
	identityModel, err := s.workerModel("coding_product_identity", specialist.RoleCodingProductIdentityStation)
	if err != nil {
		return directCodingAssembly{}, err
	}
	artifactModel, err := s.workerModel("coding_artifact_handling", specialist.RoleCodingArtifactHandlingStation)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	specification, err := runDirectCodingApplicationInterpreter(
		workerRuntime, partitionModel, surfaceModel, identityModel, artifactModel, redacted, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := validateDirectCodingRequirementCount(specification.Requirements); err != nil {
		return directCodingAssembly{}, err
	}
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
		filepath.Base(s.root), specification, identities, skills, capabilities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	generated, err := s.generateProgramFragments(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	program.Generated = generated
	if err := s.stageProgram(&program); err != nil {
		return directCodingAssembly{}, err
	}
	if err := s.recordPendingSkillCheck(
		"isolated_stage", "Complete in-memory assembly passed isolated tests, type checks, and production build.",
	); err != nil {
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
	s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_specification_accepted", fmt.Sprintf(
		"surface=%s requirements=%d product_bytes=%d",
		specification.Surface, len(specification.Requirements), len(specification.ProductQuote),
	))
	s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_assembly_ready", fmt.Sprintf(
		"adapter=%s files=%d blocks=%d waves=%d",
		program.Adapter, len(assembly.Files), blockCount, waveCount,
	))
	return assembly, nil
}

func requireDirectCodingModel(role, modelName string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("direct coding %s model is not configured", role)
	}
	return modelName, nil
}
