package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingArtifactValidationKind string

const (
	directCodingArtifactParse      directCodingArtifactValidationKind = "parse"
	directCodingArtifactStructural directCodingArtifactValidationKind = "structural_validate"
)

type directCodingArtifactValidation struct {
	Kind    directCodingArtifactValidationKind
	Execute func(path string, source []byte) error
}

// directCodingArtifactAdapter is deterministic support for one artifact class.
// It identifies a leaf, advertises only mechanics that code can really run, and
// never exposes a tool catalogue or grants the model any authority.
type directCodingArtifactAdapter struct {
	ID              string
	Validation      directCodingArtifactValidation
	Recognize       func(path string) (assemblyline.TargetArtifactKind, bool)
	ComposeDocument func(assemblyline.SourceDocument, assemblyline.SourceComposition) (assemblyline.ComposedSourceDocument, error)
}

func registeredDirectCodingArtifactAdapters() []directCodingArtifactAdapter {
	return []directCodingArtifactAdapter{
		parsedArtifactAdapter("composer_lock", composerLockArtifactRecognizer, validateJSONArtifactSource),
		parsedArtifactAdapter("typescript_react", func(value string) (assemblyline.TargetArtifactKind, bool) {
			switch {
			case strings.HasSuffix(value, ".test.tsx"):
				return assemblyline.TargetArtifactVerification, true
			case strings.HasSuffix(value, ".tsx"):
				return assemblyline.TargetArtifactImplementation, true
			default:
				return "", false
			}
		}, validateTypeScriptReactArtifactSource, assemblyline.ComposeTypeScriptDocument),
		parsedArtifactAdapter("typescript", suffixArtifactRecognizer(".ts", ".test.ts"), validateTypeScriptArtifactSource, assemblyline.ComposeTypeScriptDocument),
		parsedArtifactAdapter("go", suffixArtifactRecognizer(".go", "_test.go"), validateGoArtifactSource, assemblyline.ComposeGoDocument),
		parsedArtifactAdapter("go_module", goModuleArtifactRecognizer, validateGoModuleArtifactSource),
		parsedArtifactAdapter("php", phpArtifactRecognizer, validatePHPArtifactSource, assemblyline.ComposePHPDocument),
		parsedArtifactAdapter("php_executable", phpExecutableArtifactRecognizer, validatePHPExecutableArtifactSource),
		structuralArtifactAdapter("postgresql_migration", postgreSQLMigrationArtifactRecognizer, validatePostgreSQLMigrationArtifactSource),
		parsedArtifactAdapter("javascript", javascriptArtifactRecognizer, validateJavaScriptArtifactSource, assemblyline.ComposeJavaScriptDocument),
		structuralArtifactAdapter("css_tailwind", suffixArtifactRecognizer(".css", ""), validateCSSArtifactSource),
		parsedArtifactAdapter("html", suffixArtifactRecognizer(".html", ".test.html"), validateHTMLArtifactSource),
		parsedArtifactAdapter("java", suffixArtifactRecognizer(".java", "Test.java"), validateJavaArtifactSource, assemblyline.ComposeJavaDocument),
		parsedArtifactAdapter("rust", suffixArtifactRecognizer(".rs", "_test.rs"), validateRustArtifactSource, assemblyline.ComposeRustDocument),
		parsedArtifactAdapter("cargo_toml", cargoTOMLArtifactRecognizer, validateCargoTOMLArtifactSource),
		parsedArtifactAdapter("nginx", nginxArtifactRecognizer, validateNginxArtifactSource),
		parsedArtifactAdapter("dockerfile", dockerArtifactRecognizer, validateDockerArtifactSource),
		parsedArtifactAdapter("structured_json", suffixArtifactRecognizer(".json", ""), validateJSONArtifactSource),
		parsedArtifactAdapter("structured_yaml", yamlArtifactRecognizer, validateYAMLArtifactSource),
		parsedArtifactAdapter("environment_example", environmentArtifactRecognizer, validateEnvironmentArtifactSource),
		structuralArtifactAdapter("plain_text", plainTextArtifactRecognizer, validatePlainTextArtifactSource),
	}
}

func composerLockArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	return assemblyline.TargetArtifactImplementation, path.Base(value) == "composer.lock"
}

func goModuleArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	return assemblyline.TargetArtifactImplementation, path.Base(value) == "go.mod"
}

func cargoTOMLArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	base := path.Base(value)
	return assemblyline.TargetArtifactImplementation, base == "Cargo.toml" || base == "Cargo.lock"
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

func phpExecutableArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	return assemblyline.TargetArtifactImplementation, path.Base(value) == "artisan"
}

func postgreSQLMigrationArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	return assemblyline.TargetArtifactImplementation,
		!path.IsAbs(value) && path.Clean(value) == value &&
			strings.HasPrefix(value, "database/migrations/") &&
			strings.HasSuffix(strings.ToLower(value), ".sql") && path.Base(value) != ".sql"
}

func javascriptArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	lower := strings.ToLower(value)
	extension := path.Ext(lower)
	switch extension {
	case ".js", ".jsx", ".mjs", ".cjs":
	default:
		return "", false
	}
	base := strings.TrimSuffix(lower, extension)
	if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".spec") {
		return assemblyline.TargetArtifactVerification, true
	}
	return assemblyline.TargetArtifactImplementation, true
}

func parsedArtifactAdapter(
	id string,
	recognize func(path string) (assemblyline.TargetArtifactKind, bool),
	parseSource func(path string, source []byte) error,
	composeDocument ...func(assemblyline.SourceDocument, assemblyline.SourceComposition) (assemblyline.ComposedSourceDocument, error),
) directCodingArtifactAdapter {
	return executableArtifactAdapter(
		id, directCodingArtifactValidation{Kind: directCodingArtifactParse, Execute: parseSource},
		recognize, composeDocument...,
	)
}

func structuralArtifactAdapter(
	id string,
	recognize func(path string) (assemblyline.TargetArtifactKind, bool),
	validateStructure func(path string, source []byte) error,
) directCodingArtifactAdapter {
	return executableArtifactAdapter(
		id, directCodingArtifactValidation{
			Kind: directCodingArtifactStructural, Execute: validateStructure,
		},
		recognize,
	)
}

func executableArtifactAdapter(
	id string,
	validation directCodingArtifactValidation,
	recognize func(path string) (assemblyline.TargetArtifactKind, bool),
	composeDocument ...func(assemblyline.SourceDocument, assemblyline.SourceComposition) (assemblyline.ComposedSourceDocument, error),
) directCodingArtifactAdapter {
	adapter := directCodingArtifactAdapter{
		ID: id, Validation: validation, Recognize: recognize,
	}
	if len(composeDocument) > 1 {
		panic("artifact adapter accepts at most one document composer")
	}
	if len(composeDocument) == 1 {
		adapter.ComposeDocument = composeDocument[0]
	}
	return adapter
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
	base := path.Base(value)
	return assemblyline.TargetArtifactImplementation,
		base == ".gitignore" || base == ".dockerignore"
}

func directCodingArtifactAdapterByID(id string) (directCodingArtifactAdapter, error) {
	if err := validateDirectCodingArtifactRegistries(); err != nil {
		return directCodingArtifactAdapter{}, err
	}
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		if adapter.ID == id {
			return adapter, nil
		}
	}
	return directCodingArtifactAdapter{}, fmt.Errorf("artifact adapter %q is not registered", id)
}

func directCodingArtifactAdapterForPath(path string) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, error) {
	adapter, kind, recognized, err := recognizeDirectCodingArtifactAdapterForPath(path)
	if err != nil {
		return directCodingArtifactAdapter{}, "", err
	}
	if !recognized {
		return directCodingArtifactAdapter{}, "", fmt.Errorf("artifact path %q has no registered adapter", path)
	}
	return adapter, kind, nil
}

func recognizeDirectCodingArtifactAdapterForPath(
	path string,
) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, bool, error) {
	if err := validateDirectCodingArtifactRegistries(); err != nil {
		return directCodingArtifactAdapter{}, "", false, err
	}
	var matched directCodingArtifactAdapter
	var kind assemblyline.TargetArtifactKind
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		candidateKind, recognized := adapter.Recognize(path)
		if !recognized {
			continue
		}
		if matched.ID != "" {
			return directCodingArtifactAdapter{}, "", false, fmt.Errorf(
				"artifact path %q matches both %s and %s adapters", path, matched.ID, adapter.ID,
			)
		}
		matched, kind = adapter, candidateKind
	}
	if matched.ID == "" {
		return directCodingArtifactAdapter{}, "", false, nil
	}
	return matched, kind, true, nil
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
