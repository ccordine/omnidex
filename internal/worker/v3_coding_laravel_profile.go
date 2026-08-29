package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const (
	laravelHTTPServiceAdapter  = "laravel_http_service_capabilities_v1"
	laravelVersionProfileV1    = "laravel_http_service_versions_v1"
	laravelPHPVersion          = "8.3.30"
	laravelFrameworkVersion    = "13.29.0"
	laravelSkeletonVersion     = "13.10.1"
	laravelComposerVersion     = "2.10.2"
	laravelComposerLockSHA256  = "e1e1ad040deed5fc38b53fc7ddf1a6e125fa995f1bb117f01f103a94eb9d9333"
	laravelComposerContentHash = "07e98f8e265578d196e84517a2ad889f"

	laravelPHPImage      = "php:8.3.30-fpm-alpine@sha256:9158b5d619387f3aeb903281228edfce08cab963e1591158532cf0271d3e61cc"
	laravelPostgresImage = "postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
)

var laravelComposerDependencies = map[string]string{
	"ext-mbstring":      "*",
	"laravel/framework": "v" + laravelFrameworkVersion,
	"php":               laravelPHPVersion,
}

func registeredLaravelVersionProfile() directCodingProjectVersionProfile {
	return directCodingProjectVersionProfile{
		ID: laravelVersionProfileV1, StackID: laravelHTTPServiceAdapter,
		SourceDialect:       "PHP " + laravelPHPVersion + " function syntax",
		ParserQualification: "tree-sitter-php-0.23.11-laravel-13-profile-v1",
		ManifestPaths:       []string{"composer.json", "composer.lock"},
		ArtifactVersions: artifactVersions(
			"composer_lock", "Composer lock format with plugin API 2.9.0",
			"css_tailwind", "Tailwind CSS CLI 4.1.12",
			"dockerfile", "Dockerfile and Compose profile v1",
			"environment_example", "Laravel environment profile v1",
			"nginx", "Digest-pinned NGINX configuration",
			"php", "PHP "+laravelPHPVersion,
			"php_executable", "PHP "+laravelPHPVersion+" executable script",
			"plain_text", "UTF-8 text profile v1",
			"structured_json", "RFC 8259",
		),
		Components: versionComponents(
			"composer", laravelComposerVersion,
			"composer_image", phpServiceComposerImage,
			"composer_lock_sha256", laravelComposerLockSHA256,
			"docker_compose", directCodingDockerComposeVersion,
			"docker_engine", directCodingDockerEngineVersion,
			"laravel_framework", laravelFrameworkVersion,
			"laravel_skeleton", laravelSkeletonVersion,
			"nginx_image", phpServiceNginxImage,
			"node", directCodingPHPNodeVersion,
			"node_image", phpServiceNodeImage,
			"npm_lock", "3",
			"php", laravelPHPVersion,
			"php_image", laravelPHPImage,
			"postgres_image", laravelPostgresImage,
			"tailwindcss", "4.1.12",
		),
		ComposerDependencies: cloneStringMap(laravelComposerDependencies),
		ComposerLockTemplate: append([]byte(nil), laravelComposerLockTemplate...),
		NPMDevDependencies: map[string]string{
			"@tailwindcss/cli": "4.1.12", "tailwindcss": "4.1.12",
		},
		NPMLockTemplate:    phpServicePackageLockTemplate,
		MatchExisting:      matchLaravelVersionProfile,
		ValidateRuntime:    validatePHPRuntimeProfile,
		ValidateAssembly:   validateLaravelVersionProfileAssembly,
		ValidateDefinition: validateLaravelVersionProfile,
	}
}

func matchLaravelVersionProfile(
	profile directCodingProjectVersionProfile,
	manifests map[string]string,
) (directCodingVersionCompatibility, error) {
	manifest, exists := manifests["composer.json"]
	if !exists {
		return directCodingVersionNotApplicable, nil
	}
	lock, hasLock := manifests["composer.lock"]
	if !hasLock {
		return directCodingVersionUnsupported, nil
	}
	if err := validateLaravelComposerManifest(profile, manifest); err != nil {
		return directCodingVersionUnsupported, nil
	}
	if !bytes.Equal([]byte(lock), profile.ComposerLockTemplate) {
		return directCodingVersionUnsupported, nil
	}
	return directCodingVersionCompatible, nil
}

func validateLaravelComposerManifest(
	profile directCodingProjectVersionProfile,
	source string,
) error {
	var manifest struct {
		Name             string            `json:"name"`
		Type             string            `json:"type"`
		Require          map[string]string `json:"require"`
		RequireDev       map[string]string `json:"require-dev"`
		MinimumStability string            `json:"minimum-stability"`
		PreferStable     bool              `json:"prefer-stable"`
		Config           struct {
			AllowPlugins bool              `json:"allow-plugins"`
			Platform     map[string]string `json:"platform"`
			SortPackages bool              `json:"sort-packages"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(source), &manifest); err != nil {
		return fmt.Errorf("decode Laravel Composer manifest: %w", err)
	}
	expectedPlatform := map[string]string{
		"ext-mbstring": laravelPHPVersion,
		"php":          laravelPHPVersion,
	}
	if manifest.Name != "generated/application" || manifest.Type != "project" ||
		!reflect.DeepEqual(manifest.Require, profile.ComposerDependencies) ||
		!reflect.DeepEqual(manifest.RequireDev, profile.ComposerDevDependencies) ||
		manifest.MinimumStability != "stable" || !manifest.PreferStable ||
		manifest.Config.AllowPlugins || !manifest.Config.SortPackages ||
		!reflect.DeepEqual(manifest.Config.Platform, expectedPlatform) {
		return fmt.Errorf("Laravel Composer manifest differs from registered version profile %s", profile.ID)
	}
	return nil
}

func validateLaravelVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	files := directCodingAssemblyFiles(assembly)
	for taskID, endpoint := range program.ServiceEndpoints.ByTask {
		if err := validateLaravelReservedEndpointRoute(taskID, endpoint.RouteTemplate); err != nil {
			return err
		}
	}
	status, err := matchLaravelVersionProfile(profile, map[string]string{
		"composer.json": files["composer.json"], "composer.lock": files["composer.lock"],
	})
	if err != nil || status != directCodingVersionCompatible {
		return fmt.Errorf("Laravel assembly differs from its exact Composer profile")
	}
	for _, component := range []string{"composer_image", "nginx_image", "php_image"} {
		value, componentErr := directCodingVersionComponent(profile, component)
		if componentErr != nil {
			return componentErr
		}
		if !strings.Contains(files["Dockerfile"]+files["docker-compose.yml"], value) {
			return fmt.Errorf("Laravel assembly lacks pinned %s authority", component)
		}
	}
	storage, err := deriveDirectCodingServiceStoragePlan(program.Workload, program.ServiceState)
	if err != nil {
		return err
	}
	postgresImage, err := directCodingVersionComponent(profile, "postgres_image")
	if err != nil {
		return err
	}
	if strings.Contains(files["docker-compose.yml"], postgresImage) != storage.RequiresPostgreSQL() {
		return fmt.Errorf("Laravel PostgreSQL image differs from derived durable-state authority")
	}
	nodeImage, err := directCodingVersionComponent(profile, "node_image")
	if err != nil {
		return err
	}
	if strings.Contains(files["Dockerfile"], nodeImage) != phpServiceHasHTMLResponse(program.ServiceEndpoints) {
		return fmt.Errorf("Laravel Node image differs from accepted HTML endpoint authority")
	}
	expectedBootstrap, err := laravelBootstrapSource(profile)
	if err != nil {
		return err
	}
	if files["bootstrap/app.php"] != expectedBootstrap ||
		files["bootstrap/providers.php"] != "<?php\n\nreturn [];\n" ||
		files["artisan"] != laravelArtisanSource() ||
		files["public/index.php"] != laravelPublicIndexSource() {
		return fmt.Errorf("Laravel bootstrap artifacts differ from exact skeleton projection")
	}
	routeReadinessCount := strings.Count(files["routes/web.php"], laravelReadinessRouteSource())
	verifierReadinessCount := strings.Count(files["tests/HttpVerifier.php"],
		"performHttpRequest('GET', "+phpSingleQuoted(directCodingDeploymentReadinessPath)+", [], '')")
	composeReadinessCount := strings.Count(files["docker-compose.yml"], directCodingDeploymentReadinessPath)
	nginxHealthcheckCount := strings.Count(files["docker-compose.yml"], laravelNginxHealthcheck())
	if routeReadinessCount != 1 || verifierReadinessCount != 1 || composeReadinessCount != 1 ||
		nginxHealthcheckCount != 1 {
		return fmt.Errorf(
			"Laravel assembly readiness authority counts route=%d verifier=%d compose=%d nginx_health=%d",
			routeReadinessCount, verifierReadinessCount, composeReadinessCount, nginxHealthcheckCount,
		)
	}
	if !strings.Contains(files["Dockerfile"], laravelExactPHPAssertion()) ||
		!strings.Contains(files["Dockerfile"], "composer install --no-dev --no-interaction --no-plugins --no-scripts") ||
		!strings.Contains(files["Dockerfile"], "composer check-platform-reqs --no-dev") ||
		!strings.Contains(files["Dockerfile"], "docker-php-ext-install mbstring") {
		return fmt.Errorf("Laravel assembly lacks exact PHP or Composer install authority")
	}
	composerVersion, err := directCodingVersionComponent(profile, "composer")
	if err != nil {
		return err
	}
	if strings.Count(files["Dockerfile"], "Composer version "+composerVersion+" ") < 2 {
		return fmt.Errorf("Laravel assembly does not assert Composer %s in both build stages", composerVersion)
	}
	return validateLaravelNPMAssembly(profile, files)
}

func validateLaravelNPMAssembly(
	profile directCodingProjectVersionProfile,
	files map[string]string,
) error {
	manifest, hasManifest := files["package.json"]
	lock, hasLock := files["package-lock.json"]
	if hasManifest != hasLock {
		return fmt.Errorf("Laravel profile requires its npm manifest and lock together")
	}
	if !hasManifest {
		return nil
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	if err := validateNPMManifestVersionProfile(manifest, profile, map[string]string{"node": node}); err != nil {
		return err
	}
	return validatePinnedNPMLockForProfile(manifest, lock, profile)
}

func laravelExactPHPAssertion() string {
	return `RUN php -r 'if (PHP_VERSION !== "8.3.30") { fwrite(STDERR, "PHP runtime must be 8.3.30\n"); exit(1); }'`
}
