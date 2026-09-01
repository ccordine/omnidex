package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingProjectStack is the code-owned set of adapters that can assemble
// one complete application surface. Leaf recognition remains
// separate because a parser can exist before a greenfield stack is complete.
type directCodingProjectStack struct {
	ID                        string
	SupportedSurfaces         []assemblyline.ApplicationSurface
	ConstraintDescription     string
	ArtifactAdapterIDs        []string
	TargetTreeAdapterIDs      []string
	TargetTreeReservedPaths   []string
	ProjectCompleteTargetTree func(directCodingTargetTreeOccupation) (assemblyline.TargetTree, error)
	ProjectFocusedTargetTree  func(int, directCodingTargetTreeOccupation) (assemblyline.TargetTree, error)
	CompileSource             directCodingProjectCompiler
	ValidateBlueprint         func(assemblyline.SourceBlueprint) error
	ValidateSourceOwnership   func(
		assemblyline.FrozenApplicationWorkload,
		assemblyline.SourceBlueprint,
	) error
	RequireStagedVerification bool
	NewSourceGenerator        func(*directCodingSession, directCodingProgram) (directCodingProjectSourceGenerator, error)
}

type directCodingProjectCompiler func(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	profile directCodingProjectVersionProfile,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error)

type directCodingProjectSourceGenerator interface {
	GenerateBlock(assemblyline.ApplicationTaskContext, *directCodingProgram, assemblyline.SourceBlockRef) (string, error)
}

type directCodingProjectStageVerifier interface {
	VerifyTask(assemblyline.ApplicationTaskContext, *directCodingProgram) error
	VerifyFinal(*directCodingProgram) error
	Close() error
}

func registeredDirectCodingProjectStacks() []directCodingProjectStack {
	return []directCodingProjectStack{
		{
			ID:                    genericTypeScriptBrowserAdapter,
			SupportedSurfaces:     []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceBrowser},
			ConstraintDescription: "TypeScript with React for a browser application",
			ArtifactAdapterIDs: []string{
				"typescript_react", "typescript", "css_tailwind",
				"html", "structured_json", "plain_text",
			},
			TargetTreeAdapterIDs: []string{"typescript_react"},
			TargetTreeReservedPaths: []string{
				"src/App.tsx", "src/main.tsx", "src/runtime.tsx",
			},
			ProjectCompleteTargetTree: projectTypeScriptBrowserCompleteTargetTree,
			CompileSource:             compileGenericTypeScriptBrowserBlueprint,
			ValidateBlueprint:         assemblyline.ValidateTypeScriptSourceBlueprint,
			ValidateSourceOwnership:   validateDirectCodingSinglePairSourceOwnership,
			RequireStagedVerification: true,
			NewSourceGenerator:        newDirectCodingTypeScriptSourceGenerator,
		},
		{
			ID:                       genericGoCommandLineAdapter,
			SupportedSurfaces:        []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:    "Go with a module manifest for a command-line application",
			ArtifactAdapterIDs:       []string{"go", "go_module", "plain_text"},
			TargetTreeAdapterIDs:     []string{"go"},
			TargetTreeReservedPaths:  []string{"main.go", "runtime.go"},
			ProjectFocusedTargetTree: projectGoCommandLineFocusedTargetTree,
			CompileSource:            compileGenericGoCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateGoSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
			RequireStagedVerification: true,
			NewSourceGenerator:        newDirectCodingGoSourceGenerator,
		},
		{
			ID:                       genericJavaScriptCommandLineAdapter,
			SupportedSurfaces:        []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:    "Modern ECMAScript modules on Node.js for a command-line application",
			ArtifactAdapterIDs:       []string{"javascript", "structured_json", "plain_text"},
			TargetTreeAdapterIDs:     []string{"javascript"},
			TargetTreeReservedPaths:  []string{"main.mjs", "runtime.mjs"},
			ProjectFocusedTargetTree: projectJavaScriptCommandLineFocusedTargetTree,
			CompileSource:            compileGenericJavaScriptCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateJavaScriptSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			NewSourceGenerator:       newDirectCodingLanguageSourceGeneratorForProgram,
		},
		{
			ID:                    genericRustCommandLineAdapter,
			SupportedSurfaces:     []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription: "Rust with Cargo for a command-line application",
			ArtifactAdapterIDs:    []string{"rust", "cargo_toml", "plain_text"},
			TargetTreeAdapterIDs:  []string{"rust"},
			TargetTreeReservedPaths: []string{
				"src/lib.rs", "src/main.rs", "src/runtime.rs",
			},
			ProjectFocusedTargetTree: projectRustCommandLineFocusedTargetTree,
			CompileSource:            compileGenericRustCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateRustSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			NewSourceGenerator:       newDirectCodingLanguageSourceGeneratorForProgram,
		},
		{
			ID:                       genericJavaCommandLineAdapter,
			SupportedSurfaces:        []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:    "Java for a command-line application",
			ArtifactAdapterIDs:       []string{"java", "plain_text"},
			TargetTreeAdapterIDs:     []string{"java"},
			TargetTreeReservedPaths:  []string{"Main.java", "Runtime.java"},
			ProjectFocusedTargetTree: projectJavaCommandLineFocusedTargetTree,
			CompileSource:            compileGenericJavaCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateJavaSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			NewSourceGenerator:       newDirectCodingLanguageSourceGeneratorForProgram,
		},
	}
}

func directCodingProjectStackByID(id string) (directCodingProjectStack, error) {
	for _, stack := range registeredDirectCodingProjectStacks() {
		if stack.ID == id {
			return stack, nil
		}
	}
	return directCodingProjectStack{}, fmt.Errorf("project stack %q is not registered", id)
}

func directCodingArtifactAdapterForTreePath(
	stack directCodingProjectStack,
	artifactPath string,
) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, error) {
	adapter, kind, err := directCodingArtifactAdapterForPath(artifactPath)
	if err != nil {
		return directCodingArtifactAdapter{}, "", err
	}
	for _, allowedID := range stack.TargetTreeAdapterIDs {
		if adapter.ID == allowedID {
			return adapter, kind, nil
		}
	}
	return directCodingArtifactAdapter{}, "", fmt.Errorf(
		"target-tree file %q is not supported by selected project stack %s", artifactPath, stack.ID,
	)
}

func directCodingArtifactAdapterForProjectPath(
	stack directCodingProjectStack,
	artifactPath string,
) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, error) {
	adapter, kind, err := directCodingArtifactAdapterForPath(artifactPath)
	if err != nil {
		return directCodingArtifactAdapter{}, "", err
	}
	for _, allowedID := range stack.ArtifactAdapterIDs {
		if adapter.ID == allowedID {
			return adapter, kind, nil
		}
	}
	return directCodingArtifactAdapter{}, "", fmt.Errorf(
		"artifact %q uses adapter %s, which is not registered in project stack %s",
		artifactPath, adapter.ID, stack.ID,
	)
}
