package worker

import (
	"fmt"
	"path/filepath"

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
	resultRelationModel, err := s.workerModel(station.CodingRequirementResultRelation)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		redacted,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	interpretation, err := runDirectCodingApplicationInterpreter(
		workerRuntime,
		directCodingApplicationIntentModels{
			Requirements: requirementModel, ResultRelation: resultRelationModel,
		},
		func() (string, error) { return s.workerModel(station.CodingSurface) },
		func() (string, error) { return s.workerModel(station.CodingArtifactHandling) },
		requestAuthority, applicationContext, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	specification := interpretation.Specification
	protected, required, deletions, err := resolveDirectCodingArtifactPaths(
		specification.Artifacts, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	s.protectedPaths = directCodingProtectedPathSet(protected)
	if len(specification.Requirements) == 0 {
		return directCodingAssembly{
			RequiredPaths: append([]string(nil), required...),
			DeletePaths:   append([]string(nil), deletions...),
		}, nil
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
	requirementRelations, err := newDirectCodingApplicationTaskResultRelationPlan(
		workload, interpretation.AcceptedRequirements, requestAuthority,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	currentTargetOccupation, err := snapshotDirectCodingTargetTreeOccupation(
		s.root, selectedStack,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	targetTree, coverage, err := resolveDirectCodingTargetTree(
		specification, workload, selectedStack,
		append(append(append([]string(nil), protected...), required...), deletions...),
		currentTargetOccupation,
	)
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
	program, err := compileDirectCodingProgram(
		filepath.Base(s.root), specification, workload, capabilities,
		selection, targetTree, coverage, protected, required, deletions,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	program.RequirementRelations = requirementRelations
	program.TargetTree = targetTree
	program.Coverage = coverage
	if err := validateDirectCodingTypeScriptGreenfieldProgramRoot(s.root, program); err != nil {
		return directCodingAssembly{}, err
	}
	if err := s.bindDirectCodingProgramPathProvenance(program); err != nil {
		return directCodingAssembly{}, err
	}
	if err := s.runDirectCodingApplicationTaskLifecycle(workload, &program); err != nil {
		s.specification = &specification
		s.program = &program
		return directCodingAssembly{}, err
	}
	s.specification = &specification
	s.program = &program
	s.protectedPaths = directCodingProtectedPathSet(program.ProtectedPaths)
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	assembly.RequiredPaths = append([]string(nil), program.RequiredPaths...)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_artifact_sieve_passed", fmt.Sprintf(
		"stack=%s files=%d", selectedStack.ID, len(assembly.Files),
	))
	blockCount := directCodingSourceBlueprintBlockCount(program.Source)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_specification_accepted", fmt.Sprintf(
		"surface=%s requirements=%d product_bytes=%d",
		specification.Surface, len(specification.Requirements), len(specification.ProductQuote),
	))
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_assembly_ready", fmt.Sprintf(
		"adapter=%s files=%d blocks=%d",
		selectedStack.ID, len(assembly.Files), blockCount,
	))
	return assembly, nil
}
