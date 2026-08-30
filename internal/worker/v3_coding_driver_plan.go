package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) Assemble() (directCodingAssembly, error) {
	if err := validateDirectCodingArtifactRegistries(); err != nil {
		return directCodingAssembly{}, err
	}
	authority := s.directCodingAuthority()
	existingPaths, existingDirs, err := snapshotDirectCodingTargetTreePaths(s.root)
	if err != nil {
		return directCodingAssembly{}, err
	}
	initialPaths, err := snapshotDirectCodingInitialPaths(s.root, existingPaths)
	if err != nil {
		return directCodingAssembly{}, err
	}
	s.initialPaths = initialPaths
	existingManifests, err := snapshotDirectCodingVersionManifests(s.root, existingPaths)
	if err != nil {
		return directCodingAssembly{}, err
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(existingPaths)
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
	surfaceModel, err := s.workerModel(station.CodingSurface)
	if err != nil {
		return directCodingAssembly{}, err
	}
	requirementModel, err := s.workerModel(station.CodingRequirements)
	if err != nil {
		return directCodingAssembly{}, err
	}
	artifactModel, err := s.workerModel(station.CodingArtifactHandling)
	if err != nil {
		return directCodingAssembly{}, err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		redacted, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	interpretation, err := runDirectCodingApplicationInterpreter(
		workerRuntime, requirementModel, surfaceModel, artifactModel,
		requestAuthority, applicationContext, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	specification := interpretation.Specification
	if err := validateDirectCodingRequirementCount(specification.Requirements); err != nil {
		return directCodingAssembly{}, err
	}
	selection, err := selectDirectCodingProject(
		workerRuntime, func() (string, error) {
			return s.workerModel(station.CodingProjectStackConstraint)
		}, redacted, specification, existingManifests, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	selectedStack := selection.Stack
	continuedAvailabilityModel, err := s.workerModel(station.CodingServiceContinuedAvailability)
	if err != nil {
		return directCodingAssembly{}, err
	}
	persistenceDestinationModel, err := s.workerModel(station.CodingServicePersistenceDestination)
	if err != nil {
		return directCodingAssembly{}, err
	}
	deploymentResolution, err := resolveDirectCodingServiceDeploymentDisposition(
		workerRuntime, continuedAvailabilityModel, persistenceDestinationModel, redacted, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	deploymentDisposition := deploymentResolution.Disposition
	s.deploymentResolution = deploymentResolution
	s.deploymentDisposition = deploymentDisposition
	if deploymentDisposition == assemblyline.ApplicationServiceDeploymentPersistCurrentHost &&
		selectedStack.Deployment == nil {
		return directCodingAssembly{}, fmt.Errorf(
			"selected project stack %s has no registered persistent deployment transport",
			selectedStack.ID,
		)
	}
	if err := validateDirectCodingSelectedVersionRuntime(
		selection, directCodingSessionVersionProbe(s.runtime.ctx, s.root),
	); err != nil {
		return directCodingAssembly{}, err
	}
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
	serviceState, err := s.resolveServiceStateBeforeTargetTree(
		workerRuntime, selectedStack, workload, identities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	var targetTreeModel, targetTreeCorrectionModel string
	if selectedStack.ProjectCompleteTargetTree == nil &&
		selectedStack.ProjectFocusedTargetTree == nil {
		targetTreeModel, err = s.workerModel(station.CodingTargetTree)
		if err != nil {
			return directCodingAssembly{}, err
		}
		targetTreeCorrectionModel = targetTreeModel
	}
	targetTree, coverage, err := resolveDirectCodingTargetTree(
		workerRuntime, targetTreeModel, targetTreeCorrectionModel, redacted,
		specification, workload, selectedStack,
		existingPaths, existingDirs,
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
	targetTreeInput, err := directCodingTargetTreeInput(
		redacted, specification, workload, selectedStack, existingPaths, existingDirs,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	structureTransitions, err := assemblyline.DiffTargetTree(
		targetTreeInput, targetTree, targetTreeInput.ExistingPaths,
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
	runtimeCapabilities, err := s.selectRequirementRuntimeCapabilities(
		selectedStack, specification.ProductQuote, selectedVersionProfile.SourceDialect,
		specification.Requirements, capabilities,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if selectedStack.CompileServiceSource != nil {
		serviceState, err = s.resolveServiceStateInterfaces(
			workerRuntime, workload, capabilities, serviceState, identities,
		)
		if err != nil {
			return directCodingAssembly{}, err
		}
	}
	var program directCodingProgram
	if selectedStack.CompileServiceSource != nil {
		endpoints, endpointErr := s.resolveEndpointsForHTTPStack(
			workerRuntime, selectedStack, workload, capabilities, identities,
		)
		if endpointErr != nil {
			return directCodingAssembly{}, endpointErr
		}
		program, err = compileDirectCodingServiceProgram(
			filepath.Base(s.root), specification, identities, skills, workload, capabilities,
			targetTree, coverage, serviceState, endpoints,
		)
	} else {
		program, err = compileDirectCodingProgram(
			filepath.Base(s.root), specification, identities, skills, workload, capabilities,
			targetTree, coverage,
		)
	}
	if err != nil {
		return directCodingAssembly{}, err
	}
	program.RequirementRelations = requirementRelations
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
	program.StructureTransitions = append([]assemblyline.TargetTreeTransition(nil), structureTransitions...)
	if err := s.bindDirectCodingProgramPathProvenance(program); err != nil {
		return directCodingAssembly{}, err
	}
	cognition, err := newDirectCodingTaskCognition(s)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := cognition.Bootstrap(workload); err != nil {
		return directCodingAssembly{}, err
	}
	s.cognition = cognition
	if err := s.runDirectCodingApplicationTaskLifecycle(workload, &program); err != nil {
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
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		return directCodingAssembly{}, fmt.Errorf("validate complete in-memory artifact assembly: %w", err)
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_artifact_sieve_passed", fmt.Sprintf(
		"stack=%s files=%d", program.StackID, len(assembly.Files),
	))
	artifactGraph, err := directCodingArtifactGraphFromProgram(program, assembly)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := cognition.RecordArtifactGraph(artifactGraph); err != nil {
		return directCodingAssembly{}, err
	}
	assembly, err = directCodingOrderAssemblyFilesByArtifactGraph(assembly, artifactGraph)
	if err != nil {
		return directCodingAssembly{}, err
	}
	filesystemTransitions, err := directCodingAssemblyFilesystemTransitions(
		existingPaths, existingDirs, program.StructureTransitions, assembly,
	)
	if err != nil {
		return directCodingAssembly{}, err
	}
	for _, transition := range filesystemTransitions {
		if transition.Kind == assemblyline.TargetTreeEnsureDirectory {
			assembly.Directories = append(assembly.Directories, transition.Path)
		}
	}
	if err := assembly.normalize(); err != nil {
		return directCodingAssembly{}, err
	}
	if err := cognition.PlanTreeTransitionsWithArtifactGraph(filesystemTransitions, artifactGraph); err != nil {
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
	s.plannedDeletes = len(assembly.DeletePaths)
	blockCount, waveCount, err := directCodingProgramGraphMetrics(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_specification_accepted", fmt.Sprintf(
		"surface=%s requirements=%d product_bytes=%d",
		specification.Surface, len(specification.Requirements), len(specification.ProductQuote),
	))
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_assembly_ready", fmt.Sprintf(
		"adapter=%s files=%d blocks=%d waves=%d artifact_graph_nodes=%d artifact_graph_relations=%d",
		program.StackID, len(assembly.Files), blockCount, waveCount, len(artifactGraph.Artifacts), len(artifactGraph.Relations),
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
