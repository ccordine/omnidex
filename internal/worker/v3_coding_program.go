package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingProgram struct {
	Project              directCodingProjectSelection
	Workload             assemblyline.FrozenApplicationWorkload
	TargetTree           assemblyline.TargetTree
	Coverage             assemblyline.ApplicationFileCoveragePlan
	Source               assemblyline.SourceBlueprint
	StaticFiles          []directCodingFileTask
	Generated            map[string]string
	ProtectedPaths       []string
	RequiredPaths        []string
	DeletePaths          []string
}

func compileDirectCodingProgram(
	projectName string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	project directCodingProjectSelection,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (directCodingProgram, error) {
	moduleSegment, err := normalizeDirectCodingModuleSegment(projectName)
	if err != nil {
		return directCodingProgram{}, err
	}
	protected, required, deletions, err := resolveDirectCodingArtifactPaths(specification.Artifacts, identities)
	if err != nil {
		return directCodingProgram{}, err
	}
	stack := project.Stack
	if stack.CompileSource == nil {
		return directCodingProgram{}, fmt.Errorf(
			"project stack %s has no source compiler", stack.ID,
		)
	}
	blueprint, staticFiles, err := stack.CompileSource(
		moduleSegment, specification, workload, capabilities, project.Profile, targetTree, coverage,
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
		Project: project,
		Workload: workload, TargetTree: targetTree, Coverage: coverage,
		Source:           blueprint,
		StaticFiles:      staticFiles, Generated: map[string]string{},
		ProtectedPaths: protected, RequiredPaths: required, DeletePaths: deletions,
	}, nil
}

func resolveDirectCodingArtifactPaths(
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
) ([]string, []string, []string, error) {
	values := make(map[string]string, len(identities))
	for _, identity := range identities {
		values[identity.Token] = identity.Value
	}
	protected := make([]string, 0)
	required := make([]string, 0)
	deletions := make([]string, 0)
	seenProtected := make(map[string]struct{})
	seenRequired := make(map[string]struct{})
	seenDeletions := make(map[string]struct{})
	for _, directive := range directives {
		value, exists := values[directive.Token]
		if !exists {
			return nil, nil, nil, fmt.Errorf("semantic contract references unresolved opaque artifact %s", directive.Token)
		}
		path, err := requireExactDirectCodingPath(value)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("resolve artifact %s: %w", directive.Token, err)
		}
		switch directive.Disposition {
		case assemblyline.ArtifactProtect:
			if _, exists := seenProtected[path]; !exists {
				seenProtected[path] = struct{}{}
				protected = append(protected, path)
			}
		case assemblyline.ArtifactRequire:
			if _, exists := seenRequired[path]; !exists {
				seenRequired[path] = struct{}{}
				required = append(required, path)
			}
		case assemblyline.ArtifactForbid:
			if _, exists := seenDeletions[path]; !exists {
				seenDeletions[path] = struct{}{}
				deletions = append(deletions, path)
			}
		default:
			return nil, nil, nil, fmt.Errorf("artifact disposition %s has no filesystem consumer", directive.Disposition)
		}
	}
	return protected, required, deletions, nil
}

func normalizeDirectCodingModuleSegment(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	digest := directCodingDigest(raw)
	raw = strings.ToLower(raw)
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
		segment = "workspace-" + digest[:12]
	}
	if len(segment) > 64 {
		segment = strings.Trim(segment[:51], "-") + "-" + digest[:12]
	}
	return segment, nil
}
