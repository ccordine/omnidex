package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingArtifactCapability string

const (
	directCodingArtifactParse       directCodingArtifactCapability = "parse"
	directCodingArtifactAST         directCodingArtifactCapability = "ast"
	directCodingArtifactScope       directCodingArtifactCapability = "scope"
	directCodingArtifactTypeCheck   directCodingArtifactCapability = "typecheck"
	directCodingArtifactSyntaxCheck directCodingArtifactCapability = "syntax_check"
	directCodingArtifactProjectTest directCodingArtifactCapability = "project_test"
	directCodingArtifactRuntime     directCodingArtifactCapability = "runtime_verify"
)

// directCodingArtifactAdapter is deterministic support for one artifact class.
// It identifies a leaf, advertises only mechanics that code can really run, and
// never exposes a tool catalogue or grants the model any authority.
type directCodingArtifactAdapter struct {
	ID           string
	Capabilities []directCodingArtifactCapability
	Recognize    func(path string) (assemblyline.TargetArtifactKind, bool)
}

// directCodingProjectStack is the code-owned set of adapters that can assemble
// and verify one complete application surface. It is deliberately separate
// from leaf recognition: many artifact adapters may be useful in an existing
// project before they become a complete greenfield application stack.
type directCodingProjectStack struct {
	ID                 string
	Surface            assemblyline.ApplicationSurface
	TreeDescription    string
	ArtifactAdapterIDs []string
	ManifestPaths      []string
}

func registeredDirectCodingArtifactAdapters() []directCodingArtifactAdapter {
	return []directCodingArtifactAdapter{
		artifactAdapter("typescript_react", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactAST, directCodingArtifactScope, directCodingArtifactTypeCheck, directCodingArtifactProjectTest, directCodingArtifactRuntime}, func(value string) (assemblyline.TargetArtifactKind, bool) {
			switch {
			case strings.HasSuffix(value, ".test.tsx"):
				return assemblyline.TargetArtifactVerification, true
			case strings.HasSuffix(value, ".tsx"):
				return assemblyline.TargetArtifactImplementation, true
			default:
				return "", false
			}
		}),
		artifactAdapter("typescript", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactAST, directCodingArtifactScope, directCodingArtifactTypeCheck, directCodingArtifactProjectTest}, suffixArtifactRecognizer(".ts", ".test.ts")),
		artifactAdapter("go", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactAST, directCodingArtifactScope, directCodingArtifactTypeCheck, directCodingArtifactProjectTest}, suffixArtifactRecognizer(".go", "_test.go")),
		artifactAdapter("blade_html", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactRuntime}, suffixArtifactRecognizer(".blade.php", "")),
		artifactAdapter("php_laravel", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactSyntaxCheck, directCodingArtifactProjectTest}, phpArtifactRecognizer),
		artifactAdapter("javascript_stimulus", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactAST, directCodingArtifactSyntaxCheck, directCodingArtifactProjectTest}, suffixArtifactRecognizer(".js", ".test.js")),
		artifactAdapter("css_tailwind", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactSyntaxCheck, directCodingArtifactRuntime}, suffixArtifactRecognizer(".css", "")),
		artifactAdapter("html", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactRuntime}, suffixArtifactRecognizer(".html", ".test.html")),
		artifactAdapter("java", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactAST, directCodingArtifactTypeCheck, directCodingArtifactProjectTest}, suffixArtifactRecognizer(".java", "Test.java")),
		artifactAdapter("nginx", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactSyntaxCheck, directCodingArtifactRuntime}, nginxArtifactRecognizer),
		artifactAdapter("dockerfile", []directCodingArtifactCapability{directCodingArtifactParse, directCodingArtifactSyntaxCheck, directCodingArtifactRuntime}, dockerArtifactRecognizer),
		artifactAdapter("structured_json", []directCodingArtifactCapability{directCodingArtifactParse}, suffixArtifactRecognizer(".json", "")),
		artifactAdapter("structured_yaml", []directCodingArtifactCapability{directCodingArtifactParse}, yamlArtifactRecognizer),
		artifactAdapter("environment_example", []directCodingArtifactCapability{directCodingArtifactParse}, environmentArtifactRecognizer),
		artifactAdapter("plain_text", []directCodingArtifactCapability{directCodingArtifactParse}, plainTextArtifactRecognizer),
	}
}

func phpArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	if strings.HasSuffix(value, ".blade.php") || !strings.HasSuffix(value, ".php") {
		return "", false
	}
	if strings.HasSuffix(value, "Test.php") {
		return assemblyline.TargetArtifactVerification, true
	}
	return assemblyline.TargetArtifactImplementation, true
}

func registeredDirectCodingProjectStacks() []directCodingProjectStack {
	return []directCodingProjectStack{
		{
			ID:              genericTypeScriptBrowserAdapter,
			Surface:         assemblyline.ApplicationSurfaceBrowser,
			TreeDescription: "TypeScript React workload source (.tsx) and browser-test (.test.tsx) files",
			ArtifactAdapterIDs: []string{
				"typescript_react",
			},
			ManifestPaths: []string{"package.json"},
		},
	}
}

func artifactAdapter(
	id string,
	capabilities []directCodingArtifactCapability,
	recognize func(path string) (assemblyline.TargetArtifactKind, bool),
) directCodingArtifactAdapter {
	return directCodingArtifactAdapter{ID: id, Capabilities: append([]directCodingArtifactCapability(nil), capabilities...), Recognize: recognize}
}

func suffixArtifactRecognizer(suffix, testSuffix string) func(string) (assemblyline.TargetArtifactKind, bool) {
	return func(value string) (assemblyline.TargetArtifactKind, bool) {
		if !strings.HasSuffix(value, suffix) {
			return "", false
		}
		if testSuffix != "" && strings.HasSuffix(value, testSuffix) {
			return assemblyline.TargetArtifactVerification, true
		}
		return assemblyline.TargetArtifactImplementation, true
	}
}

func nginxArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	base := strings.ToLower(path.Base(value))
	return assemblyline.TargetArtifactImplementation, base == "nginx.conf" || strings.HasSuffix(base, ".conf") && strings.Contains(strings.ToLower(value), "nginx")
}

func dockerArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	base := strings.ToLower(path.Base(value))
	return assemblyline.TargetArtifactImplementation, base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || strings.HasPrefix(base, "docker-compose") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml"))
}

func yamlArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	base := strings.ToLower(path.Base(value))
	if strings.HasPrefix(base, "docker-compose") {
		return "", false
	}
	return assemblyline.TargetArtifactImplementation, strings.HasSuffix(value, ".yaml") || strings.HasSuffix(value, ".yml")
}

func environmentArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	base := strings.ToLower(path.Base(value))
	return assemblyline.TargetArtifactImplementation, base == ".env.example" || strings.HasSuffix(base, ".env.example")
}

// plainTextArtifactRecognizer covers code-owned textual project metadata that
// still belongs in the task-local artifact graph even though it has no richer
// language parser. It deliberately recognizes only stable text artifacts;
// an unknown workload path remains a loud adapter-selection failure.
func plainTextArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	return assemblyline.TargetArtifactImplementation, path.Base(value) == ".gitignore"
}

func directCodingProjectStackForTree(
	surface assemblyline.ApplicationSurface,
	existingPaths []string,
) (directCodingProjectStack, error) {
	stacks := make([]directCodingProjectStack, 0)
	for _, stack := range registeredDirectCodingProjectStacks() {
		if stack.Surface == surface {
			stacks = append(stacks, stack)
		}
	}
	if len(stacks) == 0 {
		return directCodingProjectStack{}, fmt.Errorf("no registered project stack supports application surface %s", surface)
	}
	if len(existingPaths) == 0 {
		if len(stacks) != 1 {
			return directCodingProjectStack{}, fmt.Errorf("empty workspace surface %s has %d registered project stacks; deterministic selection is ambiguous", surface, len(stacks))
		}
		return stacks[0], nil
	}
	matched := make([]directCodingProjectStack, 0, len(stacks))
	for _, stack := range stacks {
		if directCodingStackMatchesExistingTree(stack, existingPaths) {
			matched = append(matched, stack)
		}
	}
	if len(matched) != 1 {
		return directCodingProjectStack{}, fmt.Errorf("existing workspace surface %s matches %d registered project stacks", surface, len(matched))
	}
	return matched[0], nil
}

func directCodingStackMatchesExistingTree(stack directCodingProjectStack, existingPaths []string) bool {
	present := make(map[string]struct{}, len(existingPaths))
	for _, value := range existingPaths {
		present[value] = struct{}{}
	}
	for _, manifest := range stack.ManifestPaths {
		if _, exists := present[manifest]; exists {
			return true
		}
	}
	return false
}

func directCodingTreeTechnicalContext(
	surface assemblyline.ApplicationSurface,
	existingPaths []string,
) (directCodingProjectStack, string, error) {
	stack, err := directCodingProjectStackForTree(surface, existingPaths)
	if err != nil {
		return directCodingProjectStack{}, "", err
	}
	return stack, "Code-selected project stack: " + stack.TreeDescription + ". Return only workload-specific paths in this stack. Code-owned adapters independently supply any runtime, shell, bootstrap, manifests, styles, and their tests.", nil
}

func directCodingArtifactAdapterByID(id string) (directCodingArtifactAdapter, error) {
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		if adapter.ID == id {
			return adapter, nil
		}
	}
	return directCodingArtifactAdapter{}, fmt.Errorf("artifact adapter %q is not registered", id)
}

func directCodingArtifactAdapterForPath(path string) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, error) {
	var matched directCodingArtifactAdapter
	var kind assemblyline.TargetArtifactKind
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		candidateKind, recognized := adapter.Recognize(path)
		if !recognized {
			continue
		}
		if matched.ID != "" {
			return directCodingArtifactAdapter{}, "", fmt.Errorf("artifact path %q matches both %s and %s adapters", path, matched.ID, adapter.ID)
		}
		matched, kind = adapter, candidateKind
	}
	if matched.ID == "" {
		return directCodingArtifactAdapter{}, "", fmt.Errorf("artifact path %q has no registered adapter", path)
	}
	return matched, kind, nil
}

func directCodingProjectStackByID(id string) (directCodingProjectStack, error) {
	for _, stack := range registeredDirectCodingProjectStacks() {
		if stack.ID == id {
			return stack, nil
		}
	}
	return directCodingProjectStack{}, fmt.Errorf("project stack %q is not registered", id)
}

func directCodingArtifactAdapterForTreePath(
	stack directCodingProjectStack,
	path string,
) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, error) {
	adapter, kind, err := directCodingArtifactAdapterForPath(path)
	if err != nil {
		return directCodingArtifactAdapter{}, "", err
	}
	for _, allowedID := range stack.ArtifactAdapterIDs {
		if adapter.ID == allowedID {
			return adapter, kind, nil
		}
	}
	return directCodingArtifactAdapter{}, "", fmt.Errorf(
		"target-tree file %q is not supported by selected project stack %s", path, stack.ID,
	)
}

func directCodingRegisteredArtifactAdapterIDs() []string {
	adapters := registeredDirectCodingArtifactAdapters()
	ids := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		ids = append(ids, adapter.ID)
	}
	sort.Strings(ids)
	return ids
}
