package repository

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func validateRelativeRepositoryPath(value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("path must be non-empty and trimmed")
	}
	if strings.ContainsAny(value, "\x00\r\n\\") || filepath.IsAbs(value) {
		return fmt.Errorf("path %q is not a safe repository-relative path", value)
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path %q is not normalized inside the repository", value)
	}
	return nil
}

func sensitiveRepositoryPath(value string) bool {
	lower := strings.ToLower(value)
	base := path.Base(lower)
	if base == ".env.example" {
		return false
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	extension := path.Ext(lower)
	return extension == ".pem" || extension == ".key"
}

func languageForRepositoryPath(value string) string {
	switch strings.ToLower(path.Ext(value)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".php", ".phtml":
		return "php"
	case ".sql":
		return "sql"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}

func manifestForRepositoryPath(value string) string {
	switch path.Base(value) {
	case "go.mod":
		return "go_module"
	case "go.work":
		return "go_workspace"
	case "package.json":
		return "node_package"
	case "composer.json":
		return "php_composer"
	case "Cargo.toml":
		return "rust_cargo"
	case "pyproject.toml":
		return "python_project"
	case "pom.xml":
		return "java_maven"
	case "build.gradle", "build.gradle.kts":
		return "java_gradle"
	default:
		return ""
	}
}

func testRepositoryPath(value string) bool {
	lower := strings.ToLower(value)
	base := path.Base(lower)
	return strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") || strings.HasSuffix(base, "test.php") ||
		strings.HasPrefix(lower, "tests/") || strings.Contains(lower, "/tests/")
}

func generatedRepositoryPath(value string) bool {
	lower := strings.ToLower(value)
	base := path.Base(lower)
	return strings.HasSuffix(base, ".generated.go") || strings.HasSuffix(base, ".gen.go") ||
		strings.Contains(lower, "/generated/") || strings.HasPrefix(lower, "generated/")
}
