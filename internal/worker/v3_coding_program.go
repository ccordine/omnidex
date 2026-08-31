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
	StructureTransitions []assemblyline.TargetTreeTransition
	Source               assemblyline.SourceBlueprint
	StaticFiles          []directCodingFileTask
	Generated            map[string]string
	ProtectedPaths       []string
	DeletePaths          []string
}

func compileDirectCodingProgram(
	projectName string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (directCodingProgram, error) {
	moduleSegment, err := normalizeDirectCodingModuleSegment(projectName)
	if err != nil {
		return directCodingProgram{}, err
	}
	protected, deletions, err := resolveDirectCodingArtifactPaths(specification.Artifacts, identities)
	if err != nil {
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
	if stack.CompileSource == nil {
		return directCodingProgram{}, fmt.Errorf(
			"project stack %s has no source compiler", stack.ID,
		)
	}
	blueprint, staticFiles, err := stack.CompileSource(
		moduleSegment, specification, workload, capabilities, targetTree, coverage,
	)
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
		Source:           blueprint,
		StaticFiles:      staticFiles, Generated: map[string]string{},
		ProtectedPaths: protected, DeletePaths: deletions,
	}, nil
}

func resolveDirectCodingArtifactPaths(
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
) ([]string, []string, error) {
	values := make(map[string]string, len(identities))
	for _, identity := range identities {
		values[identity.Token] = identity.Value
	}
	protected := make([]string, 0)
	deletions := make([]string, 0)
	seenProtected := make(map[string]struct{})
	seenDeletions := make(map[string]struct{})
	for _, directive := range directives {
		value, exists := values[directive.Token]
		if !exists {
			return nil, nil, fmt.Errorf("semantic contract references unresolved opaque artifact %s", directive.Token)
		}
		path, err := normalizeDirectCodingPath(value)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve artifact %s: %w", directive.Token, err)
		}
		switch directive.Disposition {
		case assemblyline.ArtifactProtect:
			if _, exists := seenProtected[path]; !exists {
				seenProtected[path] = struct{}{}
				protected = append(protected, path)
			}
		case assemblyline.ArtifactForbid:
			if _, exists := seenDeletions[path]; !exists {
				seenDeletions[path] = struct{}{}
				deletions = append(deletions, path)
			}
		default:
			return nil, nil, fmt.Errorf("artifact disposition %s has no filesystem consumer", directive.Disposition)
		}
	}
	return protected, deletions, nil
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
