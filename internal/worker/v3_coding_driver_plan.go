package worker

import (
	"fmt"
	"path/filepath"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) Assemble() (directCodingAssembly, error) {
	frozenPlan, err := s.runtime.svc.repo.LoadFrozenCodingPlan(
		s.runtime.ctx, s.runtime.claim.Authority,
	)
	if err != nil {
		return directCodingAssembly{}, fmt.Errorf("load frozen coding plan: %w", err)
	}
	inputs, err := s.prepareApplicationInputs()
	if err != nil {
		return directCodingAssembly{}, err
	}
	approvedRequirements, err := approvedApplicationRequirementsFromFrozenPlan(
		frozenPlan, inputs.RequestAuthority.requestSHA256,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	interpretation, err := runDirectCodingApplicationInterpreter(
		inputs.Runtime,
		directCodingApplicationIntentModels{
			Requirements: inputs.RequirementModel, ResultRelation: inputs.ResultRelationModel,
		},
		func() (string, error) { return s.workerModel(station.CodingSurface) },
		func() (string, error) { return s.workerModel(station.CodingArtifactHandling) },
		inputs.RequestAuthority, inputs.ApplicationContext, approvedRequirements, inputs.Identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	specification := interpretation.Specification
	protected, required, deletions, err := resolveDirectCodingArtifactPaths(
		specification.Artifacts, inputs.Identities,
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
		inputs.Runtime, func() (string, error) {
			return s.workerModel(station.CodingProjectStackConstraint)
		}, inputs.RequestAuthority.modelRequest, specification, inputs.Identities,
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
		workload, interpretation.AcceptedRequirements, inputs.RequestAuthority,
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
	inputs.Runtime.PathProvenance = s.pathProvenance
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

func approvedApplicationRequirementsFromFrozenPlan(
	plan queue.FrozenCodingPlan,
	requestSHA256 string,
) ([]assemblyline.ApplicationRequirement, error) {
	requirements := make([]assemblyline.ApplicationRequirement, len(plan.Leaves))
	for index, leaf := range plan.Leaves {
		relation := assemblyline.ApplicationRequirementCandidateResultRelationResult{
			Schema:                   leaf.ResultRelation.Schema,
			CandidateSHA256:          leaf.ResultRelation.CandidateSHA256,
			KindReceiptSHA256:        leaf.ResultRelation.KindReceiptSHA256,
			CardinalityReceiptSHA256: leaf.ResultRelation.CardinalityReceiptSHA256,
			Relation:                 leaf.ResultRelation.Relation,
		}
		if err := relation.ValidateAcceptedFor(leaf.Leaf.Statement); err != nil {
			return nil, fmt.Errorf("frozen coding plan leaf %q: %w", leaf.Leaf.ID, err)
		}
		requirements[index] = assemblyline.ApplicationRequirement{
			ID:             fmt.Sprintf("requirement_%03d", index+1),
			Statement:      leaf.Leaf.Statement,
			RequestSHA256:  requestSHA256,
			ResultRelation: relation,
		}
	}
	return requirements, nil
}
