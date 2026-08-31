package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingProjectStack is the code-owned set of adapters that can assemble
// one complete application surface. Leaf recognition remains
// separate because a parser can exist before a greenfield stack is complete.
type directCodingProjectStack struct {
	ID                           string
	SupportedSurfaces            []assemblyline.ApplicationSurface
	DefaultSurfaces              []assemblyline.ApplicationSurface
	ConstraintDescription        string
	TreeDescription              string
	DefaultVersionProfileID      string
	ArtifactAdapterIDs           []string
	TargetTreeAdapterIDs         []string
	TargetTreeConstraints        assemblyline.TargetTreeConstraints
	TargetTreeReservedPaths      []string
	RuntimeCapabilities          func() ([]directCodingRuntimeCapability, error)
	BindRuntimeCapabilities      func(
		directCodingProgram,
		directCodingRuntimeCapabilityGraph,
	) (directCodingProgram, error)
	ProjectCompleteTargetTree func(directCodingTargetTreeOccupation) (assemblyline.TargetTree, error)
	ProjectFocusedTargetTree  func(int, directCodingTargetTreeOccupation) (assemblyline.TargetTree, error)
	CompileSource             directCodingProjectCompiler
	ValidateBlueprint       func(assemblyline.SourceBlueprint) error
	ValidateSourceOwnership func(
		assemblyline.FrozenApplicationWorkload,
		assemblyline.SourceBlueprint,
	) error
	ValidateAssembly func(assembly directCodingAssembly) error
	DeriveRelations  func(
		directCodingProgram,
		directCodingAssembly,
	) ([]directCodingArtifactPathRelation, error)
	NewSourceGenerator func(*directCodingSession, directCodingProgram) (directCodingProjectSourceGenerator, error)
}

type directCodingArtifactPathRelation struct {
	FromPath string
	ToPath   string
	Kind     assemblyline.ArtifactRelationKind
}

type directCodingProjectCompiler func(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error)

type directCodingProjectSourceGenerator interface {
	GenerateBlock(assemblyline.ApplicationTaskContext, *directCodingProgram, assemblyline.SourceBlockRef) (string, error)
}

func registeredDirectCodingProjectStacks() []directCodingProjectStack {
	return []directCodingProjectStack{
		{
			ID:                      genericTypeScriptBrowserAdapter,
			DefaultVersionProfileID: typeScriptBrowserVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceBrowser},
			DefaultSurfaces:         []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceBrowser},
			ConstraintDescription:   "TypeScript with React for a browser application",
			TreeDescription:         "exactly one TypeScript React workload source (.tsx) file",
			ArtifactAdapterIDs: []string{
				"typescript_react", "typescript", "css_tailwind",
				"html", "structured_json", "plain_text",
			},
			TargetTreeAdapterIDs: []string{"typescript_react"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 1,
			},
			TargetTreeReservedPaths: []string{
				"src/App.tsx", "src/main.tsx", "src/runtime.tsx",
			},
			ProjectCompleteTargetTree: projectTypeScriptBrowserCompleteTargetTree,
			CompileSource:             compileGenericTypeScriptBrowserBlueprint,
			ValidateBlueprint:         assemblyline.ValidateTypeScriptSourceBlueprint,
			ValidateSourceOwnership:   validateDirectCodingSingleImplementationSourceOwnership,
			ValidateAssembly:          validateTypeScriptBrowserAssembly,
			NewSourceGenerator:        newDirectCodingTypeScriptSourceGenerator,
		},
		{
			ID:                      genericGoCommandLineAdapter,
			DefaultVersionProfileID: goCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			DefaultSurfaces:         []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Go with a module manifest for a command-line application",
			TreeDescription:         "exactly one root-package Go workload source (.go) file",
			ArtifactAdapterIDs:      []string{"go", "go_module", "plain_text"},
			TargetTreeAdapterIDs:    []string{"go"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 1, RootFilesOnly: true,
			},
			TargetTreeReservedPaths:  []string{"main.go", "runtime.go"},
			ProjectFocusedTargetTree: projectGoCommandLineFocusedTargetTree,
			RuntimeCapabilities:      directCodingGoRuntimeCapabilities,
			BindRuntimeCapabilities:  bindDirectCodingGoRuntimeCapabilities,
			CompileSource:            compileGenericGoCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateGoSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			ValidateAssembly:         validateGoCommandLineAssembly,
			NewSourceGenerator:       newDirectCodingGoSourceGenerator,
		},
		{
			ID:                      genericJavaScriptCommandLineAdapter,
			DefaultVersionProfileID: javaScriptCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Modern ECMAScript modules on Node.js for a command-line application",
			TreeDescription:         "exactly one root ECMAScript workload module (.mjs)",
			ArtifactAdapterIDs:      []string{"javascript", "structured_json", "plain_text"},
			TargetTreeAdapterIDs:    []string{"javascript"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 1, RootFilesOnly: true,
			},
			TargetTreeReservedPaths:  []string{"main.mjs", "runtime.mjs"},
			ProjectFocusedTargetTree: projectJavaScriptCommandLineFocusedTargetTree,
			CompileSource:            compileGenericJavaScriptCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateJavaScriptSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			ValidateAssembly:         validateJavaScriptCommandLineAssembly,
			NewSourceGenerator:       newDirectCodingJavaScriptSourceGenerator,
		},
		{
			ID:                      genericRustCommandLineAdapter,
			DefaultVersionProfileID: rustCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Rust with Cargo for a command-line application",
			TreeDescription:         "exactly one snake_case Rust workload basename under a directory named src",
			ArtifactAdapterIDs:      []string{"rust", "cargo_toml", "plain_text"},
			TargetTreeAdapterIDs:    []string{"rust"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 1,
			},
			TargetTreeReservedPaths: []string{
				"src/lib.rs", "src/main.rs", "src/runtime.rs",
			},
			ProjectFocusedTargetTree: projectRustCommandLineFocusedTargetTree,
			CompileSource:            compileGenericRustCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateRustSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			ValidateAssembly:         validateRustCommandLineAssembly,
			NewSourceGenerator:       newDirectCodingRustSourceGenerator,
		},
		{
			ID:                      genericJavaCommandLineAdapter,
			DefaultVersionProfileID: javaCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Java for a command-line application",
			TreeDescription:         "exactly one root Java workload class (.java)",
			ArtifactAdapterIDs:      []string{"java", "plain_text"},
			TargetTreeAdapterIDs:    []string{"java"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 1, RootFilesOnly: true,
			},
			TargetTreeReservedPaths:  []string{"Main.java", "Runtime.java"},
			ProjectFocusedTargetTree: projectJavaCommandLineFocusedTargetTree,
			CompileSource:            compileGenericJavaCommandLineBlueprint,
			ValidateBlueprint:        assemblyline.ValidateJavaSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSingleImplementationSourceOwnership,
			ValidateAssembly:         validateJavaCommandLineAssembly,
			NewSourceGenerator:       newDirectCodingJavaSourceGenerator,
		},
	}
}

func directCodingTreeTechnicalContext(
	stack directCodingProjectStack,
) (string, error) {
	if _, err := directCodingProjectStackByID(stack.ID); err != nil {
		return "", err
	}
	context := "Required project structure: " + stack.TreeDescription +
		". Return only the application-specific names described here; omit runtime, shell, bootstrap, manifest, style, and infrastructure-support names."
	return context, nil
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
