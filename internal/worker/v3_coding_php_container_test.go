package worker

import (
	"strings"
	"testing"
)

func TestPHPHTTPStaticConfigurationIsPinnedAndSecretExcluding(t *testing.T) {
	program, _ := phpAcceptanceFixture(t)
	files := program.StaticFiles
	profile := requireDirectCodingVersionProfile(t, phpServiceVersionProfileV1)
	tailwindVersion := profile.NPMDevDependencies["tailwindcss"]
	packageJSON := phpServiceFileContent(t, files, "package.json")
	if !strings.Contains(packageJSON, `"@tailwindcss/cli": "`+tailwindVersion+`"`) ||
		!strings.Contains(packageJSON, `"tailwindcss": "`+tailwindVersion+`"`) {
		t.Fatal("Tailwind toolchain is not exactly pinned")
	}
	packageLock := phpServiceFileContent(t, files, "package-lock.json")
	for _, required := range []string{
		`"lockfileVersion": 3`, `"@tailwindcss/cli": "` + tailwindVersion + `"`,
		`"tailwindcss": "` + tailwindVersion + `"`, `"integrity": "sha512-`,
	} {
		if !strings.Contains(packageLock, required) {
			t.Fatalf("Tailwind lock omits immutable authority %s", required)
		}
	}
	composer := phpServiceFileContent(t, files, "composer.json")
	if !strings.Contains(composer, `"license": "proprietary"`) ||
		!strings.Contains(composer, `"php": "^8.2"`) {
		t.Fatal("Composer strict validation inputs are not bounded to the PHP 8 runtime")
	}
	assertPHPServiceDockerfileIsExact(t, files)
	assertPHPServiceDockerContextIsExact(t, files)
	assertPHPServiceHTTPRuntimeIsConfined(t, files)
	health, exists := directCodingSourceBlueprintBlock(program.Source, "application.http")
	if !exists || !strings.Contains(health.Static, "'/__omnidex/health'") ||
		!strings.Contains(health.Static, "new HttpResponse(204, 'none', '')") {
		t.Fatal("PHP runtime omits its code-owned readiness route")
	}
}

func assertPHPServiceDockerfileIsExact(t *testing.T, files []directCodingFileTask) {
	t.Helper()
	dockerfile := phpServiceFileContent(t, files, "Dockerfile")
	for _, required := range []string{
		"FROM " + phpServiceNodeImage, "FROM " + phpServiceComposerImage,
		"RUN npm ci --ignore-scripts --no-audit --no-fund",
		"COPY src/Runtime.php ./src/Runtime.php",
		"COPY src/Feature101.php ./src/Feature101.php",
		"COPY tests/Feature101Test.php ./tests/Feature101Test.php",
		"COPY tests/HttpVerifier.php ./tests/HttpVerifier.php",
		"COPY public/index.php ./public/index.php",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("Dockerfile omits exact build input %s", required)
		}
	}
	if strings.Contains(dockerfile, "npm install") || strings.Contains(dockerfile, "COPY .") ||
		strings.Contains(dockerfile, "COPY src ./src") || strings.Contains(dockerfile, "COPY tests ./tests") {
		t.Fatalf("Dockerfile contains a mutable or broad input path:\n%s", dockerfile)
	}
}

func assertPHPServiceDockerContextIsExact(t *testing.T, files []directCodingFileTask) {
	t.Helper()
	dockerignore := phpServiceFileContent(t, files, ".dockerignore")
	if !strings.HasPrefix(dockerignore, "**\n") {
		t.Fatalf(".dockerignore is not deny-first:\n%s", dockerignore)
	}
	for _, required := range []string{
		"!composer.json\n", "!package.json\n", "!package-lock.json\n",
		"!resources/styles.css\n", "!src/Runtime.php\n", "!src/Feature101.php\n",
		"!tests/TestRunner.php\n", "!tests/Feature101Test.php\n", "!public/index.php\n",
		"!tests/HttpVerifier.php\n",
	} {
		if !strings.Contains(dockerignore, required) {
			t.Fatalf(".dockerignore omits %s", required)
		}
	}
	for _, forbidden := range []string{"!src/**", "!tests/**", "credential", "secret", "*.pem"} {
		if strings.Contains(dockerignore, forbidden) {
			t.Fatalf(".dockerignore contains broad or credential-specific exception %s", forbidden)
		}
	}
	compose := phpServiceFileContent(t, files, "docker-compose.yml")
	if !strings.Contains(compose, "image: "+phpServiceNginxImage) {
		t.Fatal("NGINX runtime image is not pinned by digest")
	}
	if !strings.Contains(compose, `"${HOST_BIND_ADDRESS:-127.0.0.1}:${HOST_HTTP_PORT:-0}:80"`) ||
		strings.Contains(compose, `"8080:80"`) {
		t.Fatal("NGINX host publication is not explicitly configurable with safe defaults")
	}
	if !strings.Contains(compose, `test: ["CMD", "nginx", "-t"]`) {
		t.Fatal("NGINX runtime omits its code-owned healthcheck")
	}
	if strings.Count(compose, "restart: unless-stopped") != 2 {
		t.Fatal("request-local PHP service does not retain both runtime services")
	}
}

func assertPHPServiceHTTPRuntimeIsConfined(t *testing.T, files []directCodingFileTask) {
	t.Helper()
	runtime := phpServiceHTTPRuntimeSource()
	if !strings.Contains(runtime, "if ($candidate === $root)") ||
		!strings.Contains(runtime, "if (!is_file($candidate))") ||
		!strings.Contains(runtime, "Static asset path is not normalized") ||
		!strings.Contains(runtime, "header('Content-Type: ' . $response->media);") {
		t.Fatal("PHP runtime does not preserve root routing, exact media, and confined static pass-through")
	}
	nginx := phpServiceFileContent(t, files, "nginx/nginx.conf")
	if !strings.Contains(nginx, "resolver 127.0.0.11") ||
		!strings.Contains(nginx, "proxy_pass $application_upstream") ||
		strings.Contains(nginx, "proxy_pass http://app:8080") ||
		strings.Contains(strings.ToLower(nginx), "gateway") {
		t.Fatal("NGINX config does not defer app resolution or still pretends a caller header is trusted")
	}
	if strings.Contains(strings.ToLower(runtime), "public-gateway") {
		t.Fatal("PHP runtime still trusts an unverified gateway request header")
	}
}
