package omni

import (
	"path/filepath"
	"strings"
)

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "Go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".css":
		return "CSS"
	case ".html":
		return "HTML"
	case ".md":
		return "Markdown"
	case ".json":
		return "JSON"
	case ".zig":
		return "Zig"
	case ".rs":
		return "Rust"
	case ".php":
		return "PHP"
	default:
		return ""
	}
}

func moduleForPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) >= 2 && (parts[0] == "internal" || parts[0] == "cmd" || parts[0] == "src") {
		return strings.Join(parts[:2], "/")
	}
	if len(parts) > 1 {
		return parts[0]
	}
	return "."
}

func filePurpose(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	switch {
	case isTestPath(path):
		return "tests or validation"
	case strings.Contains(lower, "/api/"):
		return "API transport"
	case strings.Contains(lower, "/worker/"):
		return "background execution"
	case strings.Contains(lower, "migration"):
		return "database migration"
	case strings.Contains(lower, "realtime"):
		return "realtime coordination"
	default:
		return "project source"
	}
}

func modulePurpose(path string) string {
	if path == "." {
		return "repository root"
	}
	return "Owns " + strings.ReplaceAll(path, "/", " / ") + " behavior"
}

func tagsForPath(path string) []string {
	tags := []string{"path:" + path, "module:" + moduleForPath(path)}
	if isTestPath(path) {
		tags = append(tags, "tests")
	}
	return dedupeStrings(tags)
}

func manifestKind(path string) string {
	switch filepath.Base(path) {
	case "go.mod":
		return "go_module"
	case "package.json":
		return "node_package"
	default:
		return "manifest"
	}
}

func isEntrypointPath(path string) bool {
	switch filepath.Base(path) {
	case "main.go", "App.jsx", "App.tsx", "index.html", "main.tsx", "main.jsx":
		return true
	default:
		return false
	}
}

func entrypointKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".html":
		return "web"
	default:
		return "frontend"
	}
}

func isTestPath(path string) bool {
	return strings.HasSuffix(path, "_test.go") || strings.Contains(path, ".test.") || strings.Contains(path, ".spec.")
}

func verificationCommandForPath(path string, index WorkspaceIndex) string {
	if strings.HasSuffix(path, "_test.go") {
		return "go test ./..."
	}
	if containsStringValue(index.Manifests, "package.json") {
		return "npm test"
	}
	return ""
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
