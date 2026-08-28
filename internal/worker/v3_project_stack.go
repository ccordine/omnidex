package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingProjectStack is the code-owned set of adapters that can assemble
// and verify one complete application surface. Leaf recognition remains
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
	TaskStageStaticPaths         []string
	TaskStageOptionalStaticPaths []string
	ProjectTaskStaticFiles       func(directCodingProgram, directCodingProgram) ([]directCodingFileTask, error)
	ProjectFocusedTargetTree     func(int, []string) (assemblyline.TargetTree, error)
	CompileSource                directCodingProjectCompiler
	CompileServiceSource         directCodingServiceProjectCompiler
	ValidateServiceState         func(
		assemblyline.FrozenApplicationWorkload,
		directCodingServiceStatePlan,
	) error
	ValidateTargetTree      func(assemblyline.TargetTree) error
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
	VerificationCommands func(directCodingProgram) ([]testCommand, error)
	CleanupCommands      []testCommand
	Deployment           *directCodingDeploymentDescriptor
	NewStageExecutor     func(*directCodingSession, directCodingProgram) (directCodingProjectStageExecutor, error)
}

type directCodingArtifactPathRelation struct {
	FromPath string
	ToPath   string
	Kind     assemblyline.ArtifactRelationKind
}

type directCodingProjectCompiler func(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error)

type directCodingServiceProjectCompiler func(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	targetTree assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
	state directCodingServiceStatePlan,
	endpoints directCodingServiceEndpointPlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error)

type directCodingProjectStageExecutor interface {
	GenerateBlock(assemblyline.ApplicationTaskContext, *directCodingProgram, assemblyline.SourceBlockRef) (string, error)
	VerifyTask(assemblyline.ApplicationTaskContext, *directCodingProgram) error
	VerifyFinal(*directCodingProgram) error
	Close() error
}

func registeredDirectCodingProjectStacks() []directCodingProjectStack {
	return []directCodingProjectStack{
		{
			ID:                      genericTypeScriptBrowserAdapter,
			DefaultVersionProfileID: typeScriptBrowserVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceBrowser},
			DefaultSurfaces:         []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceBrowser},
			ConstraintDescription:   "TypeScript with React for a browser application",
			TreeDescription:         "exactly one TypeScript React workload source (.tsx) file and exactly one browser-test (.test.tsx) file",
			ArtifactAdapterIDs: []string{
				"typescript_react", "typescript", "css_tailwind",
				"html", "structured_json", "plain_text",
			},
			TargetTreeAdapterIDs: []string{"typescript_react"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 2,
			},
			TargetTreeReservedPaths: []string{
				"src/App.test.tsx", "src/App.tsx", "src/main.tsx",
				"src/runtime.test.tsx", "src/runtime.tsx",
			},
			TaskStageStaticPaths:    []string{"package.json", "package-lock.json", "tsconfig.json", "vite.config.ts"},
			CompileSource:           compileGenericTypeScriptBrowserBlueprint,
			ValidateTargetTree:      validateTypeScriptBrowserTargetTree,
			ValidateBlueprint:       assemblyline.ValidateTypeScriptSourceBlueprint,
			ValidateSourceOwnership: validateDirectCodingSinglePairSourceOwnership,
			ValidateAssembly:        validateTypeScriptBrowserAssembly,
			VerificationCommands:    typeScriptBrowserVerificationCommands,
			NewStageExecutor:        newDirectCodingTypeScriptProjectStageExecutor,
		},
		{
			ID:                      genericGoCommandLineAdapter,
			DefaultVersionProfileID: goCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			DefaultSurfaces:         []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Go with a module manifest for a command-line application",
			TreeDescription:         "exactly one root-package Go workload source (.go) file and exactly one root-package Go verification (_test.go) file",
			ArtifactAdapterIDs:      []string{"go", "go_module", "plain_text"},
			TargetTreeAdapterIDs:    []string{"go"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 2, RootFilesOnly: true,
			},
			TargetTreeReservedPaths:  []string{"main.go", "runtime.go"},
			ProjectFocusedTargetTree: projectGoCommandLineFocusedTargetTree,
			TaskStageStaticPaths:     []string{"go.mod"},
			CompileSource:            compileGenericGoCommandLineBlueprint,
			ValidateTargetTree:       validateGoCommandLineTargetTree,
			ValidateBlueprint:        assemblyline.ValidateGoSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
			ValidateAssembly:         validateGoCommandLineAssembly,
			VerificationCommands:     goCommandLineVerificationCommands,
			NewStageExecutor:         newDirectCodingGoProjectStageExecutor,
		},
		{
			ID:                      genericJavaScriptCommandLineAdapter,
			DefaultVersionProfileID: javaScriptCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Modern ECMAScript modules on Node.js for a command-line application",
			TreeDescription:         "exactly one root ECMAScript workload module (.mjs) and exactly one root Node test (.test.mjs)",
			ArtifactAdapterIDs:      []string{"javascript", "structured_json", "plain_text"},
			TargetTreeAdapterIDs:    []string{"javascript"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 2, RootFilesOnly: true,
			},
			TargetTreeReservedPaths:  []string{"main.mjs", "runtime.mjs"},
			ProjectFocusedTargetTree: projectJavaScriptCommandLineFocusedTargetTree,
			TaskStageStaticPaths:     []string{"package.json"},
			CompileSource:            compileGenericJavaScriptCommandLineBlueprint,
			ValidateTargetTree:       validateJavaScriptCommandLineTargetTree,
			ValidateBlueprint:        assemblyline.ValidateJavaScriptSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
			ValidateAssembly:         validateJavaScriptCommandLineAssembly,
			VerificationCommands:     javaScriptCommandLineVerificationCommands,
			NewStageExecutor:         newDirectCodingJavaScriptProjectStageExecutor,
		},
		{
			ID:                      genericRustCommandLineAdapter,
			DefaultVersionProfileID: rustCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Rust with Cargo for a command-line application",
			TreeDescription:         "exactly one src/<snake>.rs workload module and exactly one matching tests/<snake>_test.rs Cargo integration test",
			ArtifactAdapterIDs:      []string{"rust", "cargo_toml", "plain_text"},
			TargetTreeAdapterIDs:    []string{"rust"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 2,
			},
			TargetTreeReservedPaths: []string{
				"src/lib.rs", "src/main.rs", "src/runtime.rs",
			},
			ProjectFocusedTargetTree: projectRustCommandLineFocusedTargetTree,
			TaskStageStaticPaths:     []string{"Cargo.toml", "Cargo.lock"},
			CompileSource:            compileGenericRustCommandLineBlueprint,
			ValidateTargetTree:       validateRustCommandLineTargetTree,
			ValidateBlueprint:        assemblyline.ValidateRustSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
			ValidateAssembly:         validateRustCommandLineAssembly,
			VerificationCommands:     rustCommandLineVerificationCommands,
			NewStageExecutor:         newDirectCodingRustProjectStageExecutor,
		},
		{
			ID:                      genericJavaCommandLineAdapter,
			DefaultVersionProfileID: javaCommandLineVersionProfileV1,
			SupportedSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceCommandLine},
			ConstraintDescription:   "Java with strict javac verification for a command-line application",
			TreeDescription:         "exactly one root Java workload class (.java) and exactly one root Java verification class (Test.java)",
			ArtifactAdapterIDs:      []string{"java", "plain_text"},
			TargetTreeAdapterIDs:    []string{"java"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 2, RootFilesOnly: true,
			},
			TargetTreeReservedPaths:  []string{"Main.java", "Runtime.java", "TestRunner.java"},
			ProjectFocusedTargetTree: projectJavaCommandLineFocusedTargetTree,
			TaskStageStaticPaths:     []string{"TestRunner.java", "build/classes/.gitignore"},
			CompileSource:            compileGenericJavaCommandLineBlueprint,
			ValidateTargetTree:       validateJavaCommandLineTargetTree,
			ValidateBlueprint:        assemblyline.ValidateJavaSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
			ValidateAssembly:         validateJavaCommandLineAssembly,
			VerificationCommands:     javaCommandLineVerificationCommands,
			NewStageExecutor:         newDirectCodingJavaProjectStageExecutor,
		},
		{
			ID:                      genericPHPServiceAdapter,
			DefaultVersionProfileID: phpServiceVersionProfileV1,
			SupportedSurfaces: []assemblyline.ApplicationSurface{
				assemblyline.ApplicationSurfaceBrowser,
				assemblyline.ApplicationSurfaceService,
			},
			DefaultSurfaces:       []assemblyline.ApplicationSurface{assemblyline.ApplicationSurfaceService},
			ConstraintDescription: "PHP with NGINX, Docker Compose, and CSS for an HTTP service",
			TreeDescription:       "exactly one src/FeatureNNN.php workload class and one matching tests/FeatureNNNTest.php verifier per accepted behavior",
			ArtifactAdapterIDs: []string{
				"php", "css_tailwind", "nginx", "dockerfile", "postgresql_migration",
				"structured_json", "environment_example", "plain_text",
			},
			TargetTreeAdapterIDs: []string{"php"},
			TargetTreeConstraints: assemblyline.TargetTreeConstraints{
				ExactPathCount: 2,
			},
			TaskStageStaticPaths: []string{
				".gitignore", ".dockerignore", "composer.json", "Dockerfile",
				"docker-compose.yml", "nginx/nginx.conf",
			},
			TaskStageOptionalStaticPaths: []string{
				"package.json", "package-lock.json", "resources/styles.css",
				phpServiceStateMigrationPath, phpServiceStateMigrationRunner,
				phpServiceStateVerificationPath, phpServiceStateVerificationEnv,
				phpServiceStateDeploymentEnv,
			},
			ProjectTaskStaticFiles:   projectPHPServiceTaskStaticFiles,
			ProjectFocusedTargetTree: projectGenericPHPServiceFocusedTargetTree,
			CompileServiceSource:     compileGenericPHPServiceBlueprint,
			ValidateServiceState:     validatePHPServiceStateLifetime,
			ValidateTargetTree:       validateGenericPHPServiceTargetTree,
			ValidateBlueprint:        assemblyline.ValidatePHPSourceBlueprint,
			ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
			ValidateAssembly:         validateGenericPHPServiceAssembly,
			DeriveRelations:          deriveGenericPHPServiceRelations,
			VerificationCommands:     phpServiceVerificationCommands,
			CleanupCommands:          phpServiceCleanupCommands(),
			Deployment:               genericPHPDeploymentDescriptor(),
			NewStageExecutor:         newDirectCodingPHPProjectStageExecutor,
		},
		registeredLaravelProjectStack(),
	}
}

func directCodingTreeTechnicalContext(
	stack directCodingProjectStack,
) (string, error) {
	if _, err := directCodingProjectStackByID(stack.ID); err != nil {
		return "", err
	}
	context := "Code-selected project stack: " + stack.TreeDescription +
		". The expected tree contains only workload-specific file and directory names in this stack. Code-owned adapters independently supply any runtime, shell, bootstrap, manifests, styles, and their tests."
	return context, nil
}

func directCodingProjectStackByID(id string) (directCodingProjectStack, error) {
	if err := validateDirectCodingArtifactRegistries(); err != nil {
		return directCodingProjectStack{}, err
	}
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
