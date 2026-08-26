package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericPHPServiceStaticFiles(
	profile directCodingProjectVersionProfile,
	packageName string,
	endpoints directCodingServiceEndpointPlan,
	storage directCodingServiceStoragePlan,
	blueprint assemblyline.SourceBlueprint,
) ([]directCodingFileTask, error) {
	if packageName == "" {
		return nil, fmt.Errorf("PHP HTTP static files require one normalized package name")
	}
	if len(endpoints.ByTask) == 0 {
		return nil, fmt.Errorf("PHP HTTP static files require accepted endpoint authority")
	}
	sourcePaths, err := phpServiceContainerSourcePaths(blueprint)
	if err != nil {
		return nil, err
	}
	composerConstraint, err := directCodingVersionComponent(profile, "composer_php")
	if err != nil {
		return nil, err
	}
	nodeVersion, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return nil, err
	}
	composer, err := json.MarshalIndent(map[string]any{
		"name":        "generated/" + packageName,
		"description": "Generated HTTP service",
		"type":        "project",
		"license":     "proprietary",
		"require": map[string]string{
			"php": composerConstraint,
		},
		"config": map[string]any{
			"sort-packages": true,
		},
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode code-owned Composer manifest: %w", err)
	}
	hasHTML := phpServiceHasHTMLResponse(endpoints)
	hasState := storage.RequiresPostgreSQL()
	dockerfile, err := phpServiceDockerfile(profile, sourcePaths, hasHTML, hasState)
	if err != nil {
		return nil, err
	}
	compose, err := phpServiceComposeFile(profile, hasState)
	if err != nil {
		return nil, err
	}
	files := []directCodingFileTask{
		{Path: ".gitignore", Content: phpServiceGitignore(hasHTML, hasState)},
		{Path: ".dockerignore", Content: phpServiceDockerignore(sourcePaths, hasHTML, hasState)},
		{Path: "composer.json", Content: string(composer) + "\n"},
		{Path: "Dockerfile", Content: dockerfile},
		{Path: "docker-compose.yml", Content: compose},
		{Path: "nginx/nginx.conf", Content: phpServiceNginxConfig()},
	}
	if hasState {
		files = append(files,
			directCodingFileTask{Path: phpServiceStateMigrationPath, Content: phpServiceStateMigrationSQL()},
			directCodingFileTask{Path: phpServiceStateMigrationRunner, Content: phpServiceStateMigrationRunnerSource()},
			directCodingFileTask{Path: phpServiceStateVerificationPath, Content: phpServiceStateVerifierSource(storage.Namespace)},
			directCodingFileTask{Path: phpServiceStateVerificationEnv, Content: phpServiceStateVerificationEnvironment()},
			directCodingFileTask{Path: phpServiceStateDeploymentEnv, Content: phpServiceStateDeploymentEnvironment()},
		)
	}
	if !hasHTML {
		return files, nil
	}
	packageJSON, err := json.MarshalIndent(map[string]any{
		"name":    packageName,
		"private": true,
		"type":    "module",
		"engines": map[string]string{"node": nodeVersion},
		"scripts": map[string]string{
			"build:css": "tailwindcss -i resources/styles.css -o public/assets/app.css --minify",
		},
		"devDependencies": profile.NPMDevDependencies,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode code-owned Tailwind manifest: %w", err)
	}
	packageLock, err := phpServicePackageLock(profile, packageName)
	if err != nil {
		return nil, err
	}
	files = append(files,
		directCodingFileTask{Path: "package.json", Content: string(packageJSON) + "\n"},
		directCodingFileTask{Path: "package-lock.json", Content: packageLock},
		directCodingFileTask{Path: "resources/styles.css", Content: phpServiceTailwindSource()},
	)
	return files, nil
}

func phpServiceGitignore(hasHTML, hasState bool) string {
	content := "/vendor\n"
	if hasHTML {
		content = "/node_modules\n/public/assets/app.css\n" + content
	}
	if hasState {
		content += "/.env\n"
	}
	return content
}

func phpServiceTailwindSource() string {
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

func phpServiceNginxConfig() string {
	return `events {}

http {
  resolver 127.0.0.11 valid=30s;

  server {
    listen 80;
    server_name _;

    location / {
	  set $application_upstream http://app:8080;
	  proxy_pass $application_upstream;
      proxy_set_header Host $host;
      proxy_set_header X-Real-IP $remote_addr;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }
  }
}
`
}

func phpServiceHasHTMLResponse(endpoints directCodingServiceEndpointPlan) bool {
	for _, endpoint := range endpoints.ByTask {
		if endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			return true
		}
	}
	return false
}
