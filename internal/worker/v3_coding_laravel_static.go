package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericLaravelServiceStaticFiles(
	profile directCodingProjectVersionProfile,
	packageName string,
	endpoints directCodingServiceEndpointPlan,
	storage directCodingServiceStoragePlan,
	blueprint assemblyline.SourceBlueprint,
) ([]directCodingFileTask, error) {
	if packageName == "" {
		return nil, fmt.Errorf("Laravel static files require one normalized package name")
	}
	if len(endpoints.ByTask) == 0 {
		return nil, fmt.Errorf("Laravel static files require accepted endpoint authority")
	}
	hasState := storage.RequiresPostgreSQL()
	sourcePaths, err := laravelContainerSourcePaths(blueprint)
	if err != nil {
		return nil, err
	}
	sourcePaths = append(sourcePaths, laravelTestBootstrapPath, laravelPlatformVerificationPath)
	if hasState {
		sourcePaths = append(sourcePaths, phpServiceStateVerificationPath)
	}
	composer, err := laravelComposerManifest(profile)
	if err != nil {
		return nil, err
	}
	hasHTML := phpServiceHasHTMLResponse(endpoints)
	dockerfile, err := laravelDockerfile(profile, sourcePaths, hasHTML, hasState)
	if err != nil {
		return nil, err
	}
	compose, err := laravelComposeFile(profile, hasState)
	if err != nil {
		return nil, err
	}
	bootstrap, err := laravelBootstrapSource(profile)
	if err != nil {
		return nil, err
	}
	files := []directCodingFileTask{
		{Path: ".gitignore", Content: laravelGitignore(hasHTML)},
		{Path: ".dockerignore", Content: laravelDockerignore(sourcePaths, hasHTML, hasState)},
		{Path: ".env.example", Content: laravelEnvironmentExample(hasState)},
		{Path: "artisan", Content: laravelArtisanSource()},
		{Path: "bootstrap/app.php", Content: bootstrap},
		{Path: "bootstrap/providers.php", Content: "<?php\n\nreturn [];\n"},
		{Path: "composer.json", Content: composer},
		{Path: "composer.lock", Content: string(profile.ComposerLockTemplate)},
		{Path: "config/app.php", Content: laravelAppConfiguration()},
		{Path: "Dockerfile", Content: dockerfile},
		{Path: "docker-compose.yml", Content: compose},
		{Path: "nginx/nginx.conf", Content: laravelNginxConfig()},
		{Path: "public/index.php", Content: laravelPublicIndexSource()},
		{Path: laravelTestBootstrapPath, Content: laravelTestBootstrapSource()},
		{Path: laravelPlatformVerificationPath, Content: laravelPlatformVerifierSource(hasState)},
		{Path: laravelVerificationEnvPath, Content: laravelVerificationEnvironment(hasState)},
	}
	if hasState {
		migration, migrationErr := laravelServiceStateMigration(storage)
		if migrationErr != nil {
			return nil, migrationErr
		}
		verifier, verifierErr := laravelStateVerifierSource(storage.Namespace)
		if verifierErr != nil {
			return nil, verifierErr
		}
		files = append(files, directCodingFileTask{
			Path: phpServiceStateVerificationPath, Content: verifier,
		}, directCodingFileTask{
			Path: "config/database.php", Content: laravelDatabaseConfiguration(),
		}, directCodingFileTask{
			Path: laravelStateMigrationPath, Content: migration,
		})
	}
	if !hasHTML {
		return files, nil
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return nil, err
	}
	packageJSON, err := json.MarshalIndent(map[string]any{
		"name": packageName, "private": true, "type": "module",
		"engines": map[string]string{"node": node},
		"scripts": map[string]string{
			"build:css": "tailwindcss -i resources/styles.css -o public/assets/app.css --minify",
		},
		"devDependencies": profile.NPMDevDependencies,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Laravel Tailwind manifest: %w", err)
	}
	packageLock, err := phpServicePackageLock(profile, packageName)
	if err != nil {
		return nil, err
	}
	return append(files,
		directCodingFileTask{Path: "package.json", Content: string(packageJSON) + "\n"},
		directCodingFileTask{Path: "package-lock.json", Content: packageLock},
		directCodingFileTask{Path: "resources/styles.css", Content: laravelTailwindSource()},
	), nil
}

func laravelComposerManifest(profile directCodingProjectVersionProfile) (string, error) {
	manifest, err := json.MarshalIndent(map[string]any{
		"name": "generated/application", "description": "Generated Laravel HTTP service",
		"type": "project", "license": "proprietary",
		"require": profile.ComposerDependencies,
		"config": map[string]any{
			"allow-plugins": false,
			"platform": map[string]string{
				"ext-mbstring": laravelPHPVersion,
				"php":          laravelPHPVersion,
			},
			"sort-packages": true,
		},
		"minimum-stability": "stable", "prefer-stable": true,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode Laravel Composer manifest: %w", err)
	}
	source := string(manifest) + "\n"
	if err := validateLaravelComposerManifest(profile, source); err != nil {
		return "", err
	}
	return source, nil
}

func laravelGitignore(hasHTML bool) string {
	value := "/.env\n/vendor\n/storage/framework/cache/*\n/storage/framework/sessions/*\n/storage/framework/views/*\n"
	if hasHTML {
		value += "/node_modules\n/public/assets/app.css\n"
	}
	return value
}

func laravelEnvironmentExample(hasState bool) string {
	source := `APP_ENV=production
APP_DEBUG=false
APP_URL=http://localhost
APP_KEY=
`
	if !hasState {
		return source
	}
	return source + `DB_CONNECTION=pgsql
DB_HOST=db
DB_PORT=5432
DB_DATABASE=application
DB_USERNAME=application
DB_PASSWORD=
`
}

func laravelVerificationEnvironment(hasState bool) string {
	source := `APP_KEY=base64:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
HOST_BIND_ADDRESS=127.0.0.1
HOST_HTTP_PORT=0
`
	if !hasState {
		return source
	}
	return source + "DATABASE_PASSWORD=omnidex-laravel-verification-only-password\n"
}

func laravelTailwindSource() string {
	return `@import "tailwindcss";
@source "../src";

@layer base {
  :focus-visible {
    outline: 2px solid currentColor;
    outline-offset: 2px;
  }
}
`
}
