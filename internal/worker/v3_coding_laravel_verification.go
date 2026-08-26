package worker

import (
	"fmt"
	"strings"
)

const (
	laravelTestBootstrapPath        = "tests/LaravelBootstrap.php"
	laravelPlatformVerificationPath = "tests/LaravelPlatformVerifier.php"
	laravelVerificationEnvPath      = "tests/laravel-verification.env.example"
)

func laravelTestBootstrapSource() string {
	return `<?php
declare(strict_types=1);

require_once __DIR__ . '/../vendor/autoload.php';

$laravelVerificationApplication = Illuminate\Foundation\Application::configure(
    basePath: dirname(__DIR__),
)->create();
$laravelVerificationApplication->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();

$laravelVerificationKey = config('app.key');
if (!is_string($laravelVerificationKey) || trim($laravelVerificationKey) === '') {
    throw new RuntimeException('APP_KEY is required for Laravel verification.');
}
`
}

func laravelPlatformVerifierSource(hasState bool) string {
	extensions := "['ctype', 'filter', 'hash', 'mbstring', 'openssl', 'session', 'tokenizer']"
	if hasState {
		extensions = "['ctype', 'filter', 'hash', 'mbstring', 'openssl', 'PDO', 'pdo_pgsql', 'session', 'tokenizer']"
	}
	return fmt.Sprintf(`<?php
declare(strict_types=1);

require_once __DIR__ . '/LaravelBootstrap.php';

if (PHP_VERSION !== %s) {
    throw new RuntimeException('Laravel runtime PHP version differs from its exact profile.');
}
foreach (%s as $extension) {
    if (!extension_loaded($extension)) {
        throw new RuntimeException('Laravel runtime lacks required PHP extension ' . $extension . '.');
    }
}
if (Illuminate\Foundation\Application::VERSION !== %s ||
    Composer\InstalledVersions::getPrettyVersion('laravel/framework') !== %s ||
    Composer\InstalledVersions::getReference('laravel/framework') !== %s) {
    throw new RuntimeException('Laravel framework runtime differs from its exact locked profile.');
}
fwrite(STDOUT, 'Laravel platform verification passed.' . PHP_EOL);
`, phpSingleQuoted(laravelPHPVersion), extensions, phpSingleQuoted(laravelFrameworkVersion),
		phpSingleQuoted("v"+laravelFrameworkVersion),
		phpSingleQuoted("6e2c363716964d8238cee7097b258119a984f0cf"))
}

func laravelStateVerifierSource(namespace string) (string, error) {
	source := phpServiceStateVerifierSource(namespace)
	needle := "require_once __DIR__ . '/../src/Runtime.php';"
	replacement := "require_once __DIR__ . '/LaravelBootstrap.php';\n" + needle
	if !strings.Contains(source, needle) {
		return "", fmt.Errorf("shared service-state verifier lost its Runtime include boundary")
	}
	return strings.Replace(source, needle, replacement, 1), nil
}
