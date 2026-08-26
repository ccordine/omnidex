package worker

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	genericPHPServiceAdapter = "php_http_service_capabilities_v1"
	phpSourceAdapterID       = "php"
)

var phpServiceImplementationPath = regexp.MustCompile(`^src/Feature([0-9]{3})\.php$`)
var phpServiceVerificationPath = regexp.MustCompile(`^tests/Feature([0-9]{3})Test\.php$`)

func compileGenericPHPServiceBlueprint(
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
		genericPHPServiceAdapter, specification.Surface,
	); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"generic PHP HTTP stack: %w", err,
		)
	}
	if target.StackID != genericPHPServiceAdapter {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"PHP HTTP compiler received target stack %q", target.StackID,
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
	if err := validatePHPServiceStateLifetime(workload, state); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("validate PHP HTTP state lifetime: %w", err)
	}
	if err := state.ValidateInterfacesFor(workload, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("validate PHP HTTP state interfaces: %w", err)
	}
	storage, err := deriveDirectCodingServiceStoragePlan(workload, state)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("derive PHP HTTP storage plan: %w", err)
	}
	if endpoints.ProductContext != specification.ProductQuote {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"PHP HTTP endpoint product context differs from accepted specification",
		)
	}
	if err := endpoints.ValidateFor(workload); err != nil {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf("validate PHP HTTP endpoints: %w", err)
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
	documents, err := genericPHPServiceDocuments(
		specification, skills, contexts, workload, capabilities, coverage, endpoints, state, storage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	blueprint := assemblyline.SourceBlueprint{Documents: documents}
	if err := assemblyline.ValidatePHPSourceBlueprint(blueprint); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	staticFiles, err := genericPHPServiceStaticFiles(
		profile, packageName, endpoints, storage, blueprint,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return blueprint, staticFiles, nil
}

func validatePHPServiceEndpointSupport(endpoints directCodingServiceEndpointPlan) error {
	taskIDs := make([]string, 0, len(endpoints.ByTask))
	for taskID := range endpoints.ByTask {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		endpoint := endpoints.ByTask[taskID]
		switch endpoint.Exposure {
		case assemblyline.ApplicationServiceEndpointPublic:
		case assemblyline.ApplicationServiceEndpointAuthenticated:
			return fmt.Errorf(
				"PHP HTTP task %s requires authenticated exposure, but this stack has no identity-provider authority",
				taskID,
			)
		case assemblyline.ApplicationServiceEndpointInternal:
			return fmt.Errorf(
				"PHP HTTP task %s requires internal exposure, but this stack has no trusted network-source authority",
				taskID,
			)
		default:
			return fmt.Errorf("PHP HTTP task %s has unsupported exposure %q", taskID, endpoint.Exposure)
		}
	}
	return nil
}

func validateGenericPHPServiceTargetTree(target assemblyline.TargetTree) error {
	if len(target.Paths) != 2 {
		return fmt.Errorf(
			"project stack %s requires one src/FeatureNNN.php and tests/FeatureNNNTest.php pair",
			genericPHPServiceAdapter,
		)
	}
	implementation, verification := "", ""
	for _, artifactPath := range target.Paths {
		if match := phpServiceImplementationPath.FindStringSubmatch(artifactPath); match != nil {
			if implementation != "" {
				return fmt.Errorf("PHP HTTP target tree repeats its implementation leaf")
			}
			implementation = match[1]
			continue
		}
		if match := phpServiceVerificationPath.FindStringSubmatch(artifactPath); match != nil {
			if verification != "" {
				return fmt.Errorf("PHP HTTP target tree repeats its verification leaf")
			}
			verification = match[1]
			continue
		}
		return fmt.Errorf(
			"PHP HTTP target path %q must be src/FeatureNNN.php or tests/FeatureNNNTest.php",
			artifactPath,
		)
	}
	if implementation == "" || implementation != verification {
		return fmt.Errorf("PHP HTTP implementation and verification feature numbers must match")
	}
	return nil
}

func validatePHPServiceCoverage(
	target assemblyline.TargetTree,
	workload assemblyline.FrozenApplicationWorkload,
	coverage assemblyline.ApplicationFileCoveragePlan,
) error {
	if err := coverage.ValidateFor(target, workload); err != nil {
		return fmt.Errorf("validate PHP HTTP file coverage: %w", err)
	}
	for _, file := range coverage.Files {
		if len(file.TaskIDs) != 1 {
			return fmt.Errorf("PHP HTTP source %s requires exactly one task owner", file.Path)
		}
	}
	seen := make(map[string]string, len(workload.Tasks))
	for _, task := range workload.Tasks {
		pair, feature, err := phpServiceTaskPair(coverage, task.ID)
		if err != nil {
			return err
		}
		if previous, duplicate := seen[feature]; duplicate {
			return fmt.Errorf(
				"PHP HTTP tasks %s and %s share feature number %s", previous, task.ID, feature,
			)
		}
		seen[feature] = task.ID
		if path.Dir(pair.ImplementationPath) != "src" || path.Dir(pair.VerificationPath) != "tests" {
			return fmt.Errorf("PHP HTTP task %s pair escaped its code-owned directories", task.ID)
		}
	}
	return nil
}

func phpServiceTaskPair(
	coverage assemblyline.ApplicationFileCoveragePlan,
	taskID string,
) (directCodingTaskArtifactPair, string, error) {
	pair, err := directCodingTaskSinglePair(coverage, taskID)
	if err != nil {
		return directCodingTaskArtifactPair{}, "", fmt.Errorf("PHP HTTP task %s: %w", taskID, err)
	}
	target := assemblyline.TargetTree{Paths: []string{pair.ImplementationPath, pair.VerificationPath}}
	if err := validateGenericPHPServiceTargetTree(target); err != nil {
		return directCodingTaskArtifactPair{}, "", fmt.Errorf("PHP HTTP task %s: %w", taskID, err)
	}
	return pair, phpServiceImplementationPath.FindStringSubmatch(pair.ImplementationPath)[1], nil
}

func validateGenericPHPServiceAssembly(assembly directCodingAssembly) error {
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
	}
	for _, required := range []string{
		"composer.json", "Dockerfile", "docker-compose.yml", "nginx/nginx.conf",
		".gitignore", ".dockerignore", "src/Runtime.php",
		"public/index.php", "tests/HttpVerifier.php", "tests/TestRunner.php",
	} {
		if strings.TrimSpace(files[required]) == "" {
			return fmt.Errorf("PHP HTTP assembly lacks code-owned artifact %s", required)
		}
	}
	var manifest struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal([]byte(files["composer.json"]), &manifest); err != nil {
		return fmt.Errorf("decode PHP HTTP Composer manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Require["php"]) == "" {
		return fmt.Errorf("PHP HTTP Composer manifest lacks an explicit PHP runtime constraint")
	}
	hasHTML := strings.Contains(files["public/index.php"], "'text/html'")
	assetPaths := []string{"package.json", "package-lock.json", "resources/styles.css"}
	presentAssets := 0
	for _, assetPath := range assetPaths {
		if strings.TrimSpace(files[assetPath]) != "" {
			presentAssets++
		}
	}
	if hasHTML && presentAssets != len(assetPaths) {
		return fmt.Errorf("PHP HTTP HTML assembly requires one complete Tailwind toolchain")
	}
	if !hasHTML && presentAssets != 0 {
		return fmt.Errorf("PHP HTTP non-HTML assembly contains an unused Tailwind toolchain")
	}
	dockerfile := files["Dockerfile"]
	if hasHTML {
		if err := validatePinnedNPMLockForManifest(files["package.json"], files["package-lock.json"]); err != nil {
			return fmt.Errorf("PHP HTTP Tailwind dependency authority: %w", err)
		}
		for _, required := range []string{
			"FROM node:", "@sha256:",
			"npm ci --ignore-scripts --no-audit --no-fund",
			"public/assets/app.css",
		} {
			if !strings.Contains(dockerfile, required) {
				return fmt.Errorf("PHP HTTP HTML Dockerfile omits %s", required)
			}
		}
	} else {
		for _, forbidden := range []string{" AS assets", "npm ", "package.json", "resources/styles.css", "public/assets/app.css"} {
			if strings.Contains(dockerfile, forbidden) {
				return fmt.Errorf("PHP HTTP non-HTML Dockerfile contains unused asset input %s", forbidden)
			}
		}
	}
	if err := validatePHPServiceStateAssembly(files); err != nil {
		return err
	}
	return nil
}
