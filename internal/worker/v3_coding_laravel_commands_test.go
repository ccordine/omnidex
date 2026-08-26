package worker

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestLaravelProfileOwnsExactComposerAndSkeletonAuthority(t *testing.T) {
	t.Parallel()
	profile := requireDirectCodingVersionProfile(t, laravelVersionProfileV1)
	if err := profile.ValidateDefinition(profile); err != nil {
		t.Fatal(err)
	}
	if len(profile.ComposerLockTemplate) > maxV3WriteBytes {
		t.Fatalf("compact Composer lock bytes=%d limit=%d", len(profile.ComposerLockTemplate), maxV3WriteBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(profile.ComposerLockTemplate)); got != laravelComposerLockSHA256 {
		t.Fatalf("Composer lock SHA-256=%s", got)
	}
	manifest, err := laravelComposerManifest(profile)
	if err != nil {
		t.Fatal(err)
	}
	status, err := matchLaravelVersionProfile(profile, map[string]string{
		"composer.json": manifest, "composer.lock": string(profile.ComposerLockTemplate),
	})
	if err != nil || status != directCodingVersionCompatible {
		t.Fatalf("exact Laravel manifest match=%s error=%v", status, err)
	}
	tampered := append([]byte(nil), profile.ComposerLockTemplate...)
	tampered[len(tampered)-2] ^= 1
	status, err = matchLaravelVersionProfile(profile, map[string]string{
		"composer.json": manifest, "composer.lock": string(tampered),
	})
	if err != nil || status != directCodingVersionUnsupported {
		t.Fatalf("tampered Laravel lock match=%s error=%v", status, err)
	}

	program := laravelFixtureProgram(t, laravelWeatherFixtureInput())
	files := validateLaravelFixtureAssembly(t, program)
	expectedBootstrap, err := laravelBootstrapSource(profile)
	if err != nil {
		t.Fatal(err)
	}
	if files["bootstrap/app.php"] != expectedBootstrap ||
		files["artisan"] != laravelArtisanSource() ||
		files["public/index.php"] != laravelPublicIndexSource() {
		t.Fatal("Laravel skeleton projection differs from its exact registered source")
	}
	for _, required := range []string{
		"Composer version " + laravelComposerVersion + " ",
		"composer check-platform-reqs --no-dev", "docker-php-ext-install mbstring",
		"CMD [\"php-fpm\"]", " AS gateway", "COPY --from=application /app/public /app/public",
	} {
		if !strings.Contains(files["Dockerfile"], required) {
			t.Fatalf("Laravel Dockerfile omits %s", required)
		}
	}
	for _, forbidden := range []string{"artisan serve", "php -S"} {
		if strings.Contains(files["Dockerfile"]+files["docker-compose.yml"], forbidden) {
			t.Fatalf("Laravel production runtime retained %q", forbidden)
		}
	}
	for _, required := range []string{
		"target: gateway", "fastcgi_pass app:9000;",
		"try_files $uri $uri/ /index.php?$query_string;",
	} {
		if !strings.Contains(files["docker-compose.yml"]+files["nginx/nginx.conf"], required) {
			t.Fatalf("Laravel gateway runtime omits %s", required)
		}
	}
}

func TestLaravelVerificationCommandsAreExactAndStateIsolated(t *testing.T) {
	t.Parallel()
	requestLocal := laravelFixtureProgram(t, laravelWeatherFixtureInput())
	requestCommands, err := laravelVerificationCommands(requestLocal)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range append(requestCommands, laravelCleanupCommands()...) {
		if err := validateV3Command(command.Name, command.Args); err != nil {
			t.Fatalf("Laravel command %v escaped exact allowlist: %v", command.Args, err)
		}
		joined := strings.Join(command.Args, " ")
		if !strings.HasPrefix(joined, "compose --env-file "+laravelVerificationEnvPath+" ") {
			t.Fatalf("Laravel command lacks its exact verification environment: %s", joined)
		}
		for _, forbidden := range []string{" up --detach --wait db", " artisan migrate ", "StateVerifier.php"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("request-local Laravel verification retained %q: %s", forbidden, joined)
			}
		}
	}

	durable := laravelFixtureProgram(t, laravelCheckoutFixtureInput())
	durableCommands, err := laravelVerificationCommands(durable)
	if err != nil {
		t.Fatal(err)
	}
	up, migrate, write, read, runner, http := -1, -1, -1, -1, -1, -1
	resets := make([]int, 0, 3)
	for index, command := range durableCommands {
		if err := validateV3Command(command.Name, command.Args); err != nil {
			t.Fatalf("durable Laravel command %v escaped exact allowlist: %v", command.Args, err)
		}
		joined := strings.Join(command.Args, " ")
		switch joined {
		case strings.Join(laravelComposeArgs("up", "--detach", "--wait", "db"), " "):
			up = index
		case strings.Join(laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", "artisan", "migrate", "--force"), " "):
			migrate = index
		case strings.Join(laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "write"), " "):
			write = index
		case strings.Join(laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "read"), " "):
			read = index
		case strings.Join(laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "reset"), " "):
			resets = append(resets, index)
		case strings.Join(laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", "tests/TestRunner.php"), " "):
			runner = index
		case strings.Join(laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", "tests/HttpVerifier.php"), " "):
			http = index
		}
	}
	if !(up >= 0 && up < migrate && migrate < write && write < read) || len(resets) != 3 ||
		!(read < resets[0] && resets[0] < runner && runner < resets[1] &&
			resets[1] < http && http < resets[2]) {
		t.Fatalf(
			"Laravel state command order up=%d migrate=%d write=%d read=%d resets=%v runner=%d http=%d",
			up, migrate, write, read, resets, runner, http,
		)
	}
}

func TestLaravelDockerCommandMatcherRejectsUnregisteredVariants(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		laravelComposeArgs("up", "--detach", "db"),
		laravelComposeArgs("run", "--rm", "app", "php", "artisan", "migrate", "--force"),
		laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", "artisan", "route:list"),
		laravelComposeArgs("run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "erase"),
	} {
		if err := validateV3DockerCompose(args); err == nil {
			t.Fatalf("Laravel Docker matcher accepted unregistered command %v", args)
		}
	}
}
