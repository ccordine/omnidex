package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	phpServiceNodeImage     = "node:22-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32"
	phpServiceComposerImage = "composer:2@sha256:4d71c3c2109c61d5415544264b59ad4087e4c5b7244481723664138fd36d5040"
	phpServiceNginxImage    = "nginx:stable@sha256:46ccc48fbb1f5a43167f2ee2c279c122b96eec5d976e7f4e1e0780f59a51b4d6"
	phpServicePostgresImage = "postgres:16-alpine@sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685"
)

func phpServiceContainerSourcePaths(
	blueprint assemblyline.SourceBlueprint,
) ([]string, error) {
	return phpServiceContainerSourcePathsFor(blueprint, []string{
		"public/index.php", "src/Runtime.php", "tests/HttpVerifier.php", "tests/TestRunner.php",
	})
}

func phpServiceContainerSourcePathsFor(
	blueprint assemblyline.SourceBlueprint,
	required []string,
) ([]string, error) {
	seen := make(map[string]struct{})
	for _, document := range blueprint.Documents {
		artifactPath := document.Path
		if !strings.HasSuffix(artifactPath, ".php") {
			continue
		}
		if artifactPath == "" || path.IsAbs(artifactPath) || path.Clean(artifactPath) != artifactPath {
			return nil, fmt.Errorf("PHP HTTP container source path %q is not normalized", artifactPath)
		}
		switch {
		case strings.HasPrefix(artifactPath, "src/"),
			strings.HasPrefix(artifactPath, "tests/"),
			artifactPath == "public/index.php":
		default:
			return nil, fmt.Errorf("PHP HTTP container source path %q escaped registered directories", artifactPath)
		}
		seen[artifactPath] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for artifactPath := range seen {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	for _, artifactPath := range required {
		if _, exists := seen[artifactPath]; !exists {
			return nil, fmt.Errorf("PHP HTTP container source authority lacks %s", artifactPath)
		}
	}
	return paths, nil
}

func phpServiceDockerignore(sourcePaths []string, hasHTML, hasState bool) string {
	allowed := []string{"composer.json"}
	if hasHTML {
		allowed = append(allowed, "package.json", "package-lock.json", "resources", "resources/styles.css")
	}
	if hasState {
		allowed = append(allowed,
			"database", phpServiceStateMigrationRunner, "database/migrations",
			phpServiceStateMigrationPath, phpServiceStateVerificationPath,
		)
	}
	directories := make(map[string]struct{})
	for _, artifactPath := range sourcePaths {
		directories[path.Dir(artifactPath)] = struct{}{}
		allowed = append(allowed, artifactPath)
	}
	for directory := range directories {
		allowed = append(allowed, directory)
	}
	sort.Strings(allowed)
	var content strings.Builder
	content.WriteString("**\n")
	for _, artifactPath := range allowed {
		content.WriteString("!")
		content.WriteString(artifactPath)
		content.WriteByte('\n')
	}
	return content.String()
}

func phpServiceDockerfile(
	profile directCodingProjectVersionProfile,
	sourcePaths []string,
	hasHTML bool,
	hasState bool,
) (string, error) {
	composerImage, err := directCodingVersionComponent(profile, "composer_image")
	if err != nil {
		return "", err
	}
	nodeImage, err := directCodingVersionComponent(profile, "node_image")
	if err != nil {
		return "", err
	}
	phpAssertion, err := phpRuntimeVersionAssertion(profile)
	if err != nil {
		return "", err
	}
	nodeAssertion, err := nodeExactVersionAssertion(profile)
	if err != nil {
		return "", err
	}
	var assetCopies strings.Builder
	var applicationCopies strings.Builder
	for _, artifactPath := range sourcePaths {
		copyLine := "COPY " + artifactPath + " ./" + artifactPath + "\n"
		applicationCopies.WriteString(copyLine)
		if strings.HasPrefix(artifactPath, "src/") {
			assetCopies.WriteString(copyLine)
		}
	}
	if hasState {
		applicationCopies.WriteString("COPY " + phpServiceStateMigrationRunner + " ./" + phpServiceStateMigrationRunner + "\n")
		applicationCopies.WriteString("COPY " + phpServiceStateMigrationPath + " ./" + phpServiceStateMigrationPath + "\n")
		applicationCopies.WriteString("COPY " + phpServiceStateVerificationPath + " ./" + phpServiceStateVerificationPath + "\n")
	}
	stateRuntime := ""
	if hasState {
		stateRuntime = `RUN apk add --no-cache libpq \
    && apk add --no-cache --virtual .state-build-deps $PHPIZE_DEPS postgresql-dev \
    && docker-php-ext-install -j1 pdo_pgsql \
    && apk del .state-build-deps
`
	}
	application := "FROM " + composerImage + ` AS application
` + phpAssertion + `
WORKDIR /app
COPY composer.json ./
RUN composer validate --strict
` + stateRuntime + applicationCopies.String()
	if hasHTML {
		application = "FROM " + nodeImage + ` AS assets
` + nodeAssertion + `
WORKDIR /workspace
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY resources/styles.css ./resources/styles.css
` + assetCopies.String() + `RUN mkdir -p public/assets && npm run build:css

FROM ` + composerImage + ` AS application
` + phpAssertion + `
WORKDIR /app
COPY composer.json ./
RUN composer validate --strict
` + stateRuntime + applicationCopies.String() + `COPY --from=assets /workspace/public/assets/app.css ./public/assets/app.css
`
	}
	return application + `EXPOSE 8080
CMD ["php", "-S", "0.0.0.0:8080", "-t", "public", "public/index.php"]
`, nil
}

func phpServiceComposeFile(profile directCodingProjectVersionProfile, hasState bool) (string, error) {
	nginxImage, err := directCodingVersionComponent(profile, "nginx_image")
	if err != nil {
		return "", err
	}
	if !hasState {
		return `services:
  app:
    build:
      context: .
      target: application
    restart: unless-stopped
    expose:
      - "8080"
    healthcheck:
      test: ["CMD", "php", "-r", "$$s=fsockopen('127.0.0.1',8080,$$e,$$m,1); if(!$$s){exit(1);} fclose($$s);"]
      interval: 1s
      timeout: 3s
      retries: 30
  nginx:
    image: ` + nginxImage + `
    restart: unless-stopped
    depends_on:
      app:
        condition: service_healthy
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${HOST_HTTP_PORT:-0}:80"
    healthcheck:
      test: ["CMD", "nginx", "-t"]
      interval: 1s
      timeout: 3s
      retries: 30
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
`, nil
	}
	postgresImage, err := directCodingVersionComponent(profile, "postgres_image")
	if err != nil {
		return "", err
	}
	return `services:
  app:
    build:
      context: .
      target: application
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: "postgresql://service:${SERVICE_STATE_DB_PASSWORD}@postgres:5432/service"
    expose:
      - "8080"
    healthcheck:
      test: ["CMD", "php", "-r", "$$s=fsockopen('127.0.0.1',8080,$$e,$$m,1); if(!$$s){exit(1);} fclose($$s);"]
      interval: 1s
      timeout: 3s
      retries: 30
  postgres:
    image: ` + postgresImage + `
    restart: unless-stopped
    environment:
      POSTGRES_DB: service
      POSTGRES_PASSWORD: "${SERVICE_STATE_DB_PASSWORD}"
      POSTGRES_USER: service
    healthcheck:
      test: ["CMD-SHELL", "pg_isready --username=service --dbname=service"]
      interval: 2s
      timeout: 2s
      retries: 30
    volumes:
      - service_state_data:/var/lib/postgresql/data
  nginx:
    image: ` + nginxImage + `
    restart: unless-stopped
    depends_on:
      app:
        condition: service_healthy
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${HOST_HTTP_PORT:-0}:80"
    healthcheck:
      test: ["CMD", "nginx", "-t"]
      interval: 1s
      timeout: 3s
      retries: 30
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
volumes:
  service_state_data:
`, nil
}
