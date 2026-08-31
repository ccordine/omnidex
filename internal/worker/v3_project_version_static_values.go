package worker

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

func javaScriptNodeRuntimeGuard(constraint string) (string, error) {
	if !strings.HasPrefix(constraint, ">=") || strings.ContainsAny(constraint, " |") {
		return "", fmt.Errorf("JavaScript Node constraint %q is not one minimum semantic version", constraint)
	}
	minimum := normalizedSemver(strings.TrimPrefix(constraint, ">="))
	if !semver.IsValid(minimum) {
		return "", fmt.Errorf("JavaScript Node constraint %q is not semantic", constraint)
	}
	parts := strings.Split(strings.TrimPrefix(minimum, "v"), ".")
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	patch, patchErr := strconv.Atoi(parts[2])
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return "", fmt.Errorf("JavaScript Node constraint %q has invalid numeric components", constraint)
	}
	return fmt.Sprintf(`const nodeVersion = process.versions.node.split('.').map((part) => Number.parseInt(part, 10));
const nodeRuntimeCompatible = nodeVersion.length === 3 && nodeVersion.every(Number.isSafeInteger) &&
  (nodeVersion[0] > %[1]d || (nodeVersion[0] === %[1]d &&
    (nodeVersion[1] > %[2]d || (nodeVersion[1] === %[2]d && nodeVersion[2] >= %[3]d))));
if (!nodeRuntimeCompatible) {
  throw new Error('This generated application requires Node.js %[4]s.');
}`, major, minor, patch, constraint), nil
}
