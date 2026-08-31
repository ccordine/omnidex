package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) Assemble() (directCodingAssembly, error) {
	authority := s.directCodingAuthority()
	provenance, err := objectiveInstructionPathProvenance(
		s.runtime.ctx, s.root, authority,
	)
	if err != nil {
		return directCodingAssembly{}, fmt.Errorf("derive current-tree artifact provenance: %w", err)
	}
	s.pathProvenance = provenance
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority, provenance)
	if err != nil {
		return directCodingAssembly{}, err
	}
	requestAuthority, err := newDirectCodingApplicationRequestAuthority(authority, redacted)
	if err != nil {
		return directCodingAssembly{}, err
	}
	requirementModel, err := s.workerModel(station.CodingRequirements)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	workspaceState, err := objectiveWorkspaceState(s.root)
	if err != nil {
		return directCodingAssembly{}, err
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		redacted, workspaceState,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	interpretation, err := runDirectCodingApplicationInterpreter(
		workerRuntime, requirementModel,
		func() (string, error) { return s.workerModel(station.CodingSurface) },
		func() (string, error) { return s.workerModel(station.CodingArtifactHandling) },
		requestAuthority, applicationContext, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	specification := interpretation.Specification
	if len(specification.Requirements) == 0 {
		protected, required, deletions, err := resolveDirectCodingArtifactPaths(
			specification.Artifacts, identities,
		)
		if err != nil {
			return directCodingAssembly{}, err
		}
		s.protectedPaths = directCodingProtectedPathSet(protected)
		files, err := s.requiredArtifactFiles(required, nil)
		if err != nil {
			return directCodingAssembly{}, err
		}
		assembly := directCodingAssembly{Files: files, DeletePaths: deletions}
		if err := assembly.normalize(); err != nil {
			return directCodingAssembly{}, err
		}
		s.plannedFiles = len(files)
		s.plannedDeletes = len(deletions)
		return assembly, nil
	}
	if err := validateDirectCodingRequirementCount(specification.Requirements); err != nil {
		return directCodingAssembly{}, err
	}
	selection, err := selectDirectCodingProject(
		workerRuntime, func() (string, error) {
			return s.workerModel(station.CodingProjectStackConstraint)
		}, redacted, specification, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	selectedStack := selection.Stack
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		return directCodingAssembly{}, err
	}
	targetTree, coverage, err := resolveDirectCodingTargetTree(
		specification, workload, selectedStack,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	targetTree.VersionProfileID = selection.VersionProfileID
	selectedVersionProfile, err := directCodingVersionProfileForTargetTree(targetTree)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := s.bindDirectCodingTargetTreePathProvenance(targetTree); err != nil {
		return directCodingAssembly{}, err
	}
	workerRuntime.PathProvenance = s.pathProvenance
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_workload_frozen", fmt.Sprintf(
		"tasks=%d sha256=%s", len(workload.Tasks), workload.SHA256,
	))
	capabilities, err := s.deriveRequirementCapabilities(
		specification.ProductQuote, specification.Requirements,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	runtimeCapabilities, err := s.selectRequirementRuntimeCapabilities(
		selectedStack, specification.ProductQuote, selectedVersionProfile.SourceDialect,
		specification.Requirements, capabilities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	program, err := compileDirectCodingProgram(
		filepath.Base(s.root), specification, identities, workload, capabilities,
		targetTree, coverage,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	program, err = bindDirectCodingRuntimeCapabilities(
		selectedStack, program, runtimeCapabilities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if targetTree.StackID != selectedStack.ID || program.StackID != selectedStack.ID {
		return directCodingAssembly{}, fmt.Errorf(
			"project stack authority diverged: selected=%s tree=%s program=%s",
			selectedStack.ID, targetTree.StackID, program.StackID,
		)
	}
	program.TargetTree = targetTree
	program.Coverage = coverage
	if err := s.bindDirectCodingProgramPathProvenance(program); err != nil {
		return directCodingAssembly{}, err
	}
	if err := s.runDirectCodingApplicationTaskLifecycle(workload, &program); err != nil {
		return directCodingAssembly{}, err
	}
	s.specification = &specification
	s.program = &program
	s.protectedPaths = directCodingProtectedPathSet(program.ProtectedPaths)
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	assembly.Files, err = s.requiredArtifactFiles(program.RequiredPaths, assembly.Files)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		return directCodingAssembly{}, fmt.Errorf("validate compiled application artifacts: %w", err)
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_artifact_sieve_passed", fmt.Sprintf(
		"stack=%s files=%d", program.StackID, len(assembly.Files),
	))
	s.plannedFiles = len(assembly.Files)
	s.plannedDeletes = len(assembly.DeletePaths)
	blockCount := directCodingSourceBlueprintBlockCount(program.Source)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_specification_accepted", fmt.Sprintf(
		"surface=%s requirements=%d product_bytes=%d",
		specification.Surface, len(specification.Requirements), len(specification.ProductQuote),
	))
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_assembly_ready", fmt.Sprintf(
		"adapter=%s files=%d blocks=%d",
		program.StackID, len(assembly.Files), blockCount,
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
