package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingProgram struct {
	StackID              string
	VersionProfileID     string
	Workload             assemblyline.FrozenApplicationWorkload
	RequirementRelations directCodingApplicationTaskResultRelationPlan
	TargetTree           assemblyline.TargetTree
	Coverage             assemblyline.ApplicationFileCoveragePlan
	ServiceState         directCodingServiceStatePlan
	ServiceEndpoints     directCodingServiceEndpointPlan
	StructureTransitions []assemblyline.TargetTreeTransition
	Source               assemblyline.SourceBlueprint
	StaticFiles          []directCodingFileTask
	Generated            map[string]string
	ProtectedPaths       []string
}

func compileDirectCodingProgram(
	projectName string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (directCodingProgram, error) {
	return compileDirectCodingProgramWithServiceEndpoints(
		projectName, specification, identities, skills, workload, capabilities,
		targetTree, coverage, directCodingServiceStatePlan{}, directCodingServiceEndpointPlan{},
	)
}

func compileDirectCodingServiceProgram(
	projectName string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
	state directCodingServiceStatePlan,
	endpoints directCodingServiceEndpointPlan,
) (directCodingProgram, error) {
	return compileDirectCodingProgramWithServiceEndpoints(
		projectName, specification, identities, skills, workload, capabilities,
		targetTree, coverage, state, endpoints,
	)
}

func compileDirectCodingProgramWithServiceEndpoints(
	projectName string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
	state directCodingServiceStatePlan,
	endpoints directCodingServiceEndpointPlan,
) (directCodingProgram, error) {
	moduleSegment, err := normalizeDirectCodingModuleSegment(projectName)
	if err != nil {
		return directCodingProgram{}, err
	}
	protected, err := resolveDirectCodingProtectedArtifacts(specification.Artifacts, identities)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return directCodingProgram{}, err
	}
	if err := assemblyline.ValidateFrozenApplicationWorkloadFor(specification, workload); err != nil {
		return directCodingProgram{}, err
	}
	stack, err := directCodingProjectStackByID(targetTree.StackID)
	if err != nil {
		return directCodingProgram{}, err
	}
	if !stack.SupportsSurface(specification.Surface) {
		return directCodingProgram{}, fmt.Errorf(
			"selected project stack %s supports surfaces %s, not %s",
			stack.ID, directCodingProjectStackSurfaceSummary(stack.SupportedSurfaces), specification.Surface,
		)
	}
	versionProfile, err := directCodingVersionProfileForTargetTree(targetTree)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingTargetTreeUnion(stack, targetTree); err != nil {
		return directCodingProgram{}, fmt.Errorf(
			"validate %s target tree: %w", stack.ID, err,
		)
	}
	if len(workload.Tasks) == 1 {
		if err := validateDirectCodingFocusedTargetTree(stack, targetTree); err != nil {
			return directCodingProgram{}, fmt.Errorf(
				"validate %s target tree: %w", stack.ID, err,
			)
		}
	}
	if err := coverage.ValidateFor(targetTree, workload); err != nil {
		return directCodingProgram{}, fmt.Errorf("validate application file coverage: %w", err)
	}
	if len(workload.Tasks) > 1 {
		if err := validateDirectCodingCoveredFocusedTargetTrees(stack, workload, coverage); err != nil {
			return directCodingProgram{}, fmt.Errorf(
				"validate %s target tree: %w", stack.ID, err,
			)
		}
	}
	var blueprint assemblyline.SourceBlueprint
	var staticFiles []directCodingFileTask
	if stack.CompileServiceSource != nil {
		if stack.CompileSource != nil || stack.ValidateServiceState == nil {
			return directCodingProgram{}, fmt.Errorf(
				"HTTP project stack %s requires exactly one HTTP compiler and state validator", stack.ID,
			)
		}
		if err := stack.ValidateServiceState(workload, state); err != nil {
			return directCodingProgram{}, fmt.Errorf("validate service state plan: %w", err)
		}
		if err := endpoints.ValidateForCapabilities(
			workload, capabilities,
		); err != nil {
			return directCodingProgram{}, fmt.Errorf("validate service endpoint plan: %w", err)
		}
		blueprint, staticFiles, err = stack.CompileServiceSource(
			moduleSegment, specification, skills, workload, capabilities, targetTree, coverage, state, endpoints,
		)
	} else {
		if stack.CompileSource == nil {
			return directCodingProgram{}, fmt.Errorf(
				"project stack %s requires exactly one non-HTTP compiler", stack.ID,
			)
		}
		if endpoints.WorkloadSHA256 != "" || endpoints.ProductContext != "" ||
			len(endpoints.Requirements) != 0 || len(endpoints.ByTask) != 0 {
			return directCodingProgram{}, fmt.Errorf(
				"non-service project stack %s received service endpoint authority", stack.ID,
			)
		}
		if state.WorkloadSHA256 != "" || len(state.ByTask) != 0 {
			return directCodingProgram{}, fmt.Errorf(
				"non-service project stack %s received service state authority", stack.ID,
			)
		}
		blueprint, staticFiles, err = stack.CompileSource(
			moduleSegment, specification, skills, workload, capabilities, targetTree, coverage,
		)
	}
	if err != nil {
		return directCodingProgram{}, err
	}
	blueprint, err = bindDirectCodingSourceBlueprintAdapters(stack, blueprint)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingApplicationSourceOwnership(workload, blueprint); err != nil {
		return directCodingProgram{}, err
	}
	if stack.ValidateSourceOwnership == nil {
		return directCodingProgram{}, fmt.Errorf("project stack %s has no source-ownership validator", stack.ID)
	}
	if err := stack.ValidateSourceOwnership(workload, blueprint); err != nil {
		return directCodingProgram{}, fmt.Errorf("validate %s source ownership: %w", stack.ID, err)
	}
	return directCodingProgram{
		StackID: stack.ID, VersionProfileID: versionProfile.ID,
		Workload: workload, TargetTree: targetTree, Coverage: coverage,
		ServiceState:     state,
		ServiceEndpoints: endpoints,
		Source:           blueprint,
		StaticFiles:      staticFiles, Generated: map[string]string{},
		ProtectedPaths: protected,
	}, nil
}

func resolveDirectCodingProtectedArtifacts(
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	values := make(map[string]string, len(identities))
	for _, identity := range identities {
		values[identity.Token] = identity.Value
	}
	protected := make([]string, 0)
	seen := make(map[string]struct{})
	for _, directive := range directives {
		value, exists := values[directive.Token]
		if !exists {
			return nil, fmt.Errorf("semantic contract references unresolved opaque artifact %s", directive.Token)
		}
		if directive.Disposition != assemblyline.ArtifactProtect {
			return nil, fmt.Errorf("generic coding adapters do not support artifact disposition %s for %s", directive.Disposition, directive.Token)
		}
		path, err := normalizeDirectCodingPath(value)
		if err != nil {
			return nil, fmt.Errorf("resolve protected artifact %s: %w", directive.Token, err)
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		protected = append(protected, path)
	}
	return protected, nil
}

func normalizeDirectCodingModuleSegment(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var output strings.Builder
	lastDash := false
	for _, char := range raw {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			output.WriteRune(char)
			lastDash = false
			continue
		}
		if !lastDash && output.Len() > 0 {
			output.WriteByte('-')
			lastDash = true
		}
	}
	segment := strings.Trim(output.String(), "-")
	if segment == "" {
		return "", fmt.Errorf("project module requires a name containing a letter or digit")
	}
	return segment, nil
}
