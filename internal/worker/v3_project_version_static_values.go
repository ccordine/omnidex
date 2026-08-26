package worker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

var phpRuntimeRangePattern = regexp.MustCompile(`^>=([0-9]+)\.([0-9]+),<([0-9]+)$`)

func javaScriptNodeRuntimeGuard(constraint string) (string, error) {
	if !strings.HasPrefix(constraint, ">=") || strings.ContainsAny(constraint, " |") {
		return "", fmt.Errorf("JavaScript Node constraint %q is not one minimum semantic version", constraint)
	}
	minimum := normalizedSemver(strings.TrimPrefix(constraint, ">="))
	if !semver.IsValid(minimum) {
		return "", fmt.Errorf("JavaScript Node constraint %q is not semantic", constraint)
	}
	parts := strings.Split(strings.TrimPrefix(minimum, "v"), ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	return fmt.Sprintf(`const nodeVersion = process.versions.node.split('.').map((part) => Number.parseInt(part, 10));
const nodeRuntimeCompatible = nodeVersion.length === 3 && nodeVersion.every(Number.isSafeInteger) &&
  (nodeVersion[0] > %[1]d || (nodeVersion[0] === %[1]d &&
    (nodeVersion[1] > %[2]d || (nodeVersion[1] === %[2]d && nodeVersion[2] >= %[3]d))));
if (!nodeRuntimeCompatible) {
  throw new Error('This generated application requires Node.js %[4]s.');
}`, major, minor, patch, constraint), nil
}

func phpRuntimeVersionAssertion(profile directCodingProjectVersionProfile) (string, error) {
	constraint, err := directCodingVersionComponent(profile, "php_runtime")
	if err != nil {
		return "", err
	}
	parts := phpRuntimeRangePattern.FindStringSubmatch(constraint)
	if len(parts) != 4 {
		return "", fmt.Errorf("PHP runtime constraint %q is outside the executable range grammar", constraint)
	}
	major, _ := strconv.Atoi(parts[1])
	minor, _ := strconv.Atoi(parts[2])
	upperMajor, _ := strconv.Atoi(parts[3])
	if major < 1 || minor > 99 || upperMajor <= major {
		return "", fmt.Errorf("PHP runtime constraint %q has invalid bounds", constraint)
	}
	lowerID := major*10000 + minor*100
	upperID := upperMajor * 10000
	return fmt.Sprintf(
		`RUN php -r 'if (PHP_VERSION_ID < %d || PHP_VERSION_ID >= %d) { fwrite(STDERR, "PHP runtime must be %s\n"); exit(1); }'`,
		lowerID, upperID, constraint,
	), nil
}

func nodeExactVersionAssertion(profile directCodingProjectVersionProfile) (string, error) {
	version, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return "", err
	}
	if !semver.IsValid(normalizedSemver(version)) || strings.HasPrefix(version, "v") {
		return "", fmt.Errorf("Node container version %q must be one exact semantic version", version)
	}
	return fmt.Sprintf(
		`RUN node -e "if (process.versions.node !== '%s') { throw new Error('Node runtime must be %s'); }"`,
		version, version,
	), nil
}
