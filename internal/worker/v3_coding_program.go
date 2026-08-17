package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingProgram struct {
	Adapter                 string
	PackageName             string
	Workload                assemblyline.FrozenApplicationWorkload
	TargetTree              assemblyline.TargetTree
	StructureTransitions    []assemblyline.TargetTreeTransition
	TypeScript              assemblyline.TypeScriptBlueprint
	StaticFiles             []directCodingFileTask
	Generated               map[string]string
	AcceptanceGrounding     map[string]assemblyline.ApplicationAcceptanceGroundingReceipt
	AcceptanceGroundingSeen map[string]map[string]struct{}
	ProtectedPaths          []string
}

func compileDirectCodingProgram(
	projectName string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
) (directCodingProgram, error) {
	moduleSegment, err := normalizeDirectCodingModuleSegment(projectName)
	if err != nil {
		return directCodingProgram{}, err
	}
	protected, err := resolveDirectCodingProtectedArtifacts(specification.Artifacts, identities)
	if err != nil {
		return directCodingProgram{}, err
	}
	if specification.Surface != assemblyline.ApplicationSurfaceBrowser {
		return directCodingProgram{}, fmt.Errorf(
			"no generic coding adapter supports application surface=%s",
			specification.Surface,
		)
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return directCodingProgram{}, err
	}
	adapter, blueprint, staticFiles, err := compileGenericTypeScriptBrowserBlueprint(
		moduleSegment, specification, skills, workload, capabilities, targetTree,
	)
	if err != nil {
		return directCodingProgram{}, err
	}
	return directCodingProgram{
		Adapter: adapter, PackageName: moduleSegment, Workload: workload, TypeScript: blueprint,
		StaticFiles: staticFiles, Generated: map[string]string{},
		AcceptanceGrounding:     map[string]assemblyline.ApplicationAcceptanceGroundingReceipt{},
		AcceptanceGroundingSeen: map[string]map[string]struct{}{},
		ProtectedPaths:          protected,
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
		return "", fmt.Errorf("browser package requires a project name containing a letter or digit")
	}
	return segment, nil
}
