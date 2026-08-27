package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func registeredLaravelProjectStack() directCodingProjectStack {
	return directCodingProjectStack{
		ID:                      laravelHTTPServiceAdapter,
		DefaultVersionProfileID: laravelVersionProfileV1,
		SupportedSurfaces: []assemblyline.ApplicationSurface{
			assemblyline.ApplicationSurfaceBrowser,
			assemblyline.ApplicationSurfaceService,
		},
		ConstraintDescription: "Laravel 13 with server rendering, PostgreSQL, NGINX, and Docker Compose for an HTTP service",
		TreeDescription:       "exactly one src/FeatureNNN.php bounded workload function and one matching tests/FeatureNNNTest.php verifier per accepted behavior",
		ArtifactAdapterIDs: []string{
			"composer_lock", "css_tailwind", "dockerfile", "environment_example", "nginx", "php", "php_executable",
			"plain_text", "structured_json",
		},
		TargetTreeAdapterIDs: []string{"php"},
		TargetTreeConstraints: assemblyline.TargetTreeConstraints{
			ExactPathCount: 2,
		},
		TaskStageStaticPaths: []string{
			".dockerignore", ".env.example", ".gitignore", "Dockerfile", "artisan",
			"bootstrap/app.php", "bootstrap/providers.php", "composer.json", "composer.lock",
			"config/app.php", "docker-compose.yml", "nginx/nginx.conf", "public/index.php",
			laravelTestBootstrapPath, laravelPlatformVerificationPath,
			laravelVerificationEnvPath,
		},
		TaskStageOptionalStaticPaths: []string{
			"package.json", "package-lock.json", "resources/styles.css",
			"config/database.php", laravelStateMigrationPath, phpServiceStateVerificationPath,
		},
		ProjectTaskStaticFiles:   projectLaravelTaskStaticFiles,
		ProjectFocusedTargetTree: projectGenericPHPServiceFocusedTargetTree,
		ExclusiveTaskPaths:       true,
		CompileServiceSource:     compileGenericLaravelServiceBlueprint,
		ValidateServiceState:     validateLaravelServiceStateLifetime,
		ValidateTargetTree:       validateGenericPHPServiceTargetTree,
		ValidateBlueprint:        assemblyline.ValidatePHPSourceBlueprint,
		ValidateSourceOwnership:  validateDirectCodingSinglePairSourceOwnership,
		ValidateAssembly:         validateGenericLaravelServiceAssembly,
		DeriveRelations:          deriveGenericLaravelServiceRelations,
		VerificationCommands:     laravelVerificationCommands,
		CleanupCommands:          laravelCleanupCommands(),
		Deployment:               laravelDeploymentDescriptor(),
		NewStageExecutor:         newDirectCodingLaravelProjectStageExecutor,
	}
}

func compileGenericLaravelServiceBlueprint(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
	state directCodingServiceStatePlan,
	endpoints directCodingServiceEndpointPlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	if err := specification.Validate(); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingProjectStackSurface(
		laravelHTTPServiceAdapter, specification.Surface,
	); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"Laravel HTTP stack: %w", err,
		)
	}
	if target.StackID != laravelHTTPServiceAdapter {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"Laravel HTTP compiler received target stack %q", target.StackID,
		)
	}
	profile, err := directCodingVersionProfileForTargetTree(target)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := state.ValidateInterfacesFor(workload, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("validate Laravel state interfaces: %w", err)
	}
	storage, err := deriveDirectCodingServiceStoragePlan(workload, state)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("derive Laravel service storage: %w", err)
	}
	if endpoints.ProductContext != specification.ProductQuote {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"Laravel endpoint product context differs from accepted specification",
		)
	}
	if err := endpoints.ValidateFor(workload); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("validate Laravel endpoints: %w", err)
	}
	if err := validatePHPServiceEndpointSupport(endpoints); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validatePHPServiceCoverage(target, workload, coverage); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(applicationWorkloadInput(specification), workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	base, err := genericPHPServiceDocuments(
		specification, skills, contexts, workload, capabilities, coverage, endpoints, state, storage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents, err := laravelServiceDocuments(
		base, specification, workload, capabilities, coverage, endpoints, state, storage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	blueprint := assemblyline.SourceBlueprint{Documents: documents}
	if err := assemblyline.ValidatePHPSourceBlueprint(blueprint); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	staticFiles, err := genericLaravelServiceStaticFiles(
		profile, packageName, endpoints, storage, blueprint,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return blueprint, staticFiles, nil
}

func validateLaravelServiceStateLifetime(
	workload assemblyline.FrozenApplicationWorkload,
	plan directCodingServiceStatePlan,
) error {
	_, err := deriveDirectCodingServiceStoragePlan(workload, plan)
	return err
}

func validateGenericLaravelServiceAssembly(assembly directCodingAssembly) error {
	files := directCodingAssemblyFiles(assembly)
	for _, required := range []string{
		".env.example", "Dockerfile", "artisan", "bootstrap/app.php", "composer.json",
		"composer.lock", "docker-compose.yml", "nginx/nginx.conf", "public/index.php", "routes/web.php",
		"src/Runtime.php", laravelTestBootstrapPath, laravelPlatformVerificationPath,
		"tests/HttpVerifier.php", "tests/TestRunner.php",
	} {
		if strings.TrimSpace(files[required]) == "" {
			return fmt.Errorf("Laravel assembly lacks code-owned artifact %s", required)
		}
	}
	var manifest struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal([]byte(files["composer.json"]), &manifest); err != nil {
		return fmt.Errorf("decode Laravel Composer manifest: %w", err)
	}
	if manifest.Require["laravel/framework"] != "v"+laravelFrameworkVersion {
		return fmt.Errorf("Laravel assembly does not pin framework v%s", laravelFrameworkVersion)
	}
	for _, required := range []string{
		"Application::configure", "->withRouting(", "routes/web.php",
	} {
		if !strings.Contains(files["bootstrap/app.php"], required) {
			return fmt.Errorf("Laravel bootstrap omits %s", required)
		}
	}
	if !strings.Contains(files["routes/web.php"], "Route::") ||
		!strings.Contains(files["public/index.php"], "->handleRequest(") {
		return fmt.Errorf("Laravel request lifecycle is not authoritative")
	}
	if !strings.Contains(files["docker-compose.yml"],
		"${HOST_BIND_ADDRESS:-127.0.0.1}:${HOST_HTTP_PORT:-0}:80") {
		return fmt.Errorf("Laravel Compose port authority is not configurable and collision-free")
	}
	if !strings.Contains(files["docker-compose.yml"], "${APP_KEY:?APP_KEY is required}") ||
		!strings.Contains(files[laravelTestBootstrapPath], "APP_KEY is required") {
		return fmt.Errorf("Laravel application key authority is not explicit")
	}
	for _, marker := range []string{
		"CMD [\"php-fpm\"]", "FROM " + phpServiceNginxImage + " AS gateway",
		"target: gateway", "fastcgi_pass app:9000;",
	} {
		if !strings.Contains(
			files["Dockerfile"]+files["docker-compose.yml"]+files["nginx/nginx.conf"], marker,
		) {
			return fmt.Errorf("Laravel production runtime omits %s", marker)
		}
	}
	if strings.Contains(files["Dockerfile"]+files["docker-compose.yml"], "artisan serve") {
		return fmt.Errorf("Laravel production runtime retained the development server")
	}
	if !strings.Contains(files["src/Runtime.php"], "error_log('Unhandled Laravel route failure [' .") ||
		!strings.Contains(files["src/Runtime.php"], "report($failure);") {
		return fmt.Errorf("Laravel unexpected failures are not reported before sanitization")
	}
	stateful := strings.Contains(files["src/Runtime.php"], "final class RuntimeState")
	statePaths := []string{"config/database.php", laravelStateMigrationPath, phpServiceStateVerificationPath}
	for _, artifactPath := range statePaths {
		present := strings.TrimSpace(files[artifactPath]) != ""
		if present != stateful {
			return fmt.Errorf("Laravel durable-state artifact %s differs from runtime authority", artifactPath)
		}
	}
	stateMarkers := []string{
		"  db:", "service-state:/var/lib/postgresql", "${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}",
	}
	for _, marker := range stateMarkers {
		if strings.Contains(files["docker-compose.yml"], marker) != stateful {
			return fmt.Errorf("Laravel Compose state marker %q differs from storage authority", marker)
		}
	}
	for _, marker := range []string{"pdo_pgsql", "libpq=18.6-r0"} {
		if strings.Contains(files["Dockerfile"], marker) != stateful {
			return fmt.Errorf("Laravel image state marker %q differs from storage authority", marker)
		}
	}
	if stateful {
		migration := files[laravelStateMigrationPath]
		if migration != laravelServiceStateMigrationSource() {
			return fmt.Errorf("Laravel migration differs from canonical transactional state schema")
		}
		if !strings.Contains(files[phpServiceStateVerificationPath],
			"require_once __DIR__ . '/LaravelBootstrap.php';") {
			return fmt.Errorf("Laravel state verifier does not boot the framework")
		}
	}
	return nil
}
