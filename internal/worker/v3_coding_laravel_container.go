package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func laravelContainerSourcePaths(blueprint assemblyline.SourceBlueprint) ([]string, error) {
	return laravelContainerSourcePathsFor(blueprint, []string{
		"routes/web.php", "src/Runtime.php", "tests/HttpVerifier.php", "tests/TestRunner.php",
	})
}

func laravelContainerSourcePathsFor(
	blueprint assemblyline.SourceBlueprint,
	requiredPaths []string,
) ([]string, error) {
	required := make(map[string]bool, len(requiredPaths))
	for _, artifactPath := range requiredPaths {
		required[artifactPath] = false
	}
	seen := make(map[string]struct{})
	for _, document := range blueprint.Documents {
		artifactPath := document.Path
		if !strings.HasSuffix(artifactPath, ".php") || artifactPath == "" ||
			path.IsAbs(artifactPath) || path.Clean(artifactPath) != artifactPath {
			return nil, fmt.Errorf("Laravel container source path %q is not normalized PHP", artifactPath)
		}
		switch {
		case strings.HasPrefix(artifactPath, "src/"),
			strings.HasPrefix(artifactPath, "tests/"), artifactPath == "routes/web.php":
		default:
			return nil, fmt.Errorf("Laravel source path %q escaped registered directories", artifactPath)
		}
		seen[artifactPath] = struct{}{}
		if _, exists := required[artifactPath]; exists {
			required[artifactPath] = true
		}
	}
	for artifactPath, present := range required {
		if !present {
			return nil, fmt.Errorf("Laravel source authority lacks %s", artifactPath)
		}
	}
	paths := make([]string, 0, len(seen))
	for artifactPath := range seen {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	return paths, nil
}

func laravelDockerignore(sourcePaths []string, hasHTML, hasState bool) string {
	allowed := []string{
		".env.example", "artisan", "bootstrap", "bootstrap/app.php", "bootstrap/providers.php",
		"composer.json", "composer.lock", "config", "config/app.php",
		"public", "public/index.php",
	}
	if hasState {
		allowed = append(allowed, "config/database.php", "database", "database/migrations", laravelStateMigrationPath)
	}
	if hasHTML {
		allowed = append(allowed, "package.json", "package-lock.json", "resources", "resources/styles.css")
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

func laravelDockerfile(
	profile directCodingProjectVersionProfile,
	sourcePaths []string,
	hasHTML bool,
	hasState bool,
) (string, error) {
	composerImage, err := directCodingVersionComponent(profile, "composer_image")
	if err != nil {
		return "", err
	}
	composerVersion, err := directCodingVersionComponent(profile, "composer")
	if err != nil {
		return "", err
	}
	phpImage, err := directCodingVersionComponent(profile, "php_image")
	if err != nil {
		return "", err
	}
	nginxImage, err := directCodingVersionComponent(profile, "nginx_image")
	if err != nil {
		return "", err
	}
	nodeImage, err := directCodingVersionComponent(profile, "node_image")
	if err != nil {
		return "", err
	}
	nodeAssertion, err := nodeExactVersionAssertion(profile)
	if err != nil {
		return "", err
	}
	var sourceCopies, assetCopies strings.Builder
	for _, artifactPath := range sourcePaths {
		line := "COPY --chown=www-data:www-data " + artifactPath + " ./" + artifactPath + "\n"
		sourceCopies.WriteString(line)
		if strings.HasPrefix(artifactPath, "src/") {
			assetCopies.WriteString("COPY " + artifactPath + " ./" + artifactPath + "\n")
		}
	}
	assets := ""
	assetCopy := ""
	if hasHTML {
		assets = "FROM " + nodeImage + ` AS assets
` + nodeAssertion + `
WORKDIR /workspace
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY resources/styles.css ./resources/styles.css
` + assetCopies.String() + `RUN mkdir -p public/assets && npm run build:css

`
		assetCopy = "COPY --from=assets --chown=www-data:www-data /workspace/public/assets/app.css ./public/assets/app.css\n"
	}
	extensionInstall := laravelPHPExtensionInstall(hasState)
	databaseCopy := ""
	if hasState {
		databaseCopy = "COPY --chown=www-data:www-data database ./database\n"
	}
	return assets + "FROM " + composerImage + ` AS vendor
RUN composer --version --no-ansi | grep -F 'Composer version ` + composerVersion + ` '
WORKDIR /workspace
COPY composer.json composer.lock ./
RUN composer validate --no-check-publish && composer install --no-dev --no-interaction --no-plugins --no-scripts --prefer-dist --no-progress --classmap-authoritative

FROM ` + phpImage + ` AS application
` + laravelExactPHPAssertion() + `
COPY --from=vendor /usr/bin/composer /usr/local/bin/composer
` + extensionInstall + `
WORKDIR /app
COPY --from=vendor --chown=www-data:www-data /workspace/vendor ./vendor
COPY --chown=www-data:www-data .env.example ./
COPY --chown=www-data:www-data artisan ./artisan
COPY --chown=www-data:www-data bootstrap ./bootstrap
COPY --chown=www-data:www-data composer.json composer.lock ./
COPY --chown=www-data:www-data config ./config
` + databaseCopy + `COPY --chown=www-data:www-data public/index.php ./public/index.php
` + sourceCopies.String() + assetCopy + `RUN mkdir -p bootstrap/cache storage/framework/cache storage/framework/sessions storage/framework/views && \
    chown -R www-data:www-data bootstrap/cache storage && \
    composer --version --no-ansi | grep -F 'Composer version ` + composerVersion + ` ' && \
    composer check-platform-reqs --no-dev
USER www-data
EXPOSE 9000
CMD ["php-fpm"]

FROM ` + nginxImage + ` AS gateway
COPY --from=application /app/public /app/public
`, nil
}

func laravelPHPExtensionInstall(hasState bool) string {
	runtimePackages := "oniguruma=6.9.10-r0"
	buildPackages := "oniguruma-dev=6.9.10-r0"
	extensions := "mbstring"
	checks := "php -m | grep -Fx mbstring"
	if hasState {
		runtimePackages += " libpq=18.6-r0"
		buildPackages += " libpq-dev=18.6-r0"
		extensions += " pdo_pgsql"
		checks += " && php -m | grep -Fx pdo_pgsql"
	}
	return `RUN apk add --no-cache ` + runtimePackages + ` && \
    apk add --no-cache --virtual .php-build-deps \
      autoconf=2.72-r1 dpkg=1.22.21-r0 dpkg-dev=1.22.21-r0 file=5.46-r2 \
      g++=15.2.0-r2 gcc=15.2.0-r2 make=4.4.1-r3 musl-dev=1.2.5-r23 \
      pkgconf=2.5.1-r0 re2c=4.3.1-r0 ` + buildPackages + ` && \
    docker-php-ext-install ` + extensions + ` && apk del .php-build-deps && \
    ` + checks
}
