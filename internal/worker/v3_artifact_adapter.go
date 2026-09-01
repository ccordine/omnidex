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
	ID               string
	Validation       directCodingArtifactValidation
	Recognize        func(path string) (assemblyline.TargetArtifactKind, bool)
	ComposeDocument  func(assemblyline.SourceDocument, assemblyline.SourceComposition) (assemblyline.ComposedSourceDocument, error)
	SourceLanguage   string
	ValidateFragment directCodingLanguageFragmentValidator
}

func registeredDirectCodingArtifactAdapters() []directCodingArtifactAdapter {
	return []directCodingArtifactAdapter{
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
		sourceArtifactAdapter(
			"go", "go", suffixArtifactRecognizer(".go", "_test.go"),
			assemblyline.ComposeGoDocument, validateGoArtifactSource,
			validateDirectCodingGoFragment,
		),
		parsedArtifactAdapter("go_module", goModuleArtifactRecognizer, validateGoModuleArtifactSource),
		sourceArtifactAdapter(
			"javascript", "javascript", javascriptArtifactRecognizer,
			assemblyline.ComposeJavaScriptDocument, validateJavaScriptArtifactSource,
			validateDirectCodingJavaScriptFragment,
		),
		structuralArtifactAdapter("css_tailwind", suffixArtifactRecognizer(".css", ""), validateCSSArtifactSource),
		parsedArtifactAdapter("html", suffixArtifactRecognizer(".html", ".test.html"), validateHTMLArtifactSource),
		sourceArtifactAdapter(
			"java", "java", suffixArtifactRecognizer(".java", "Test.java"),
			assemblyline.ComposeJavaDocument, validateJavaArtifactSource,
			validateDirectCodingJavaFragment,
		),
		sourceArtifactAdapter(
			"rust", "rust", suffixArtifactRecognizer(".rs", "_test.rs"),
			assemblyline.ComposeRustDocument, validateRustArtifactSource,
			validateDirectCodingRustFragment,
		),
		parsedArtifactAdapter("cargo_toml", cargoTOMLArtifactRecognizer, validateCargoTOMLArtifactSource),
		parsedArtifactAdapter("structured_json", suffixArtifactRecognizer(".json", ""), validateJSONArtifactSource),
		structuralArtifactAdapter(
			assemblyline.PlainTextAdapterID, plainTextArtifactRecognizer,
			validatePlainTextArtifactSource, assemblyline.ComposePlainTextDocument,
		),
	}
}

func sourceArtifactAdapter(
	id string,
	language string,
	recognize func(path string) (assemblyline.TargetArtifactKind, bool),
	compose func(assemblyline.SourceDocument, assemblyline.SourceComposition) (assemblyline.ComposedSourceDocument, error),
	parseSource func(path string, source []byte) error,
	validate directCodingLanguageFragmentValidator,
) directCodingArtifactAdapter {
	adapter := parsedArtifactAdapter(id, recognize, parseSource, compose)
	adapter.SourceLanguage = language
	adapter.ValidateFragment = validate
	return adapter
}

func goModuleArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	return assemblyline.TargetArtifactImplementation, path.Base(value) == "go.mod"
}

func cargoTOMLArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	base := path.Base(value)
	return assemblyline.TargetArtifactImplementation, base == "Cargo.toml" || base == "Cargo.lock"
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
		id,
		directCodingArtifactValidation{Kind: directCodingArtifactParse, Execute: parseSource},
		recognize,
		composeDocument...,
	)
}

func structuralArtifactAdapter(
	id string,
	recognize func(path string) (assemblyline.TargetArtifactKind, bool),
	validateStructure func(path string, source []byte) error,
	composeDocument ...func(assemblyline.SourceDocument, assemblyline.SourceComposition) (assemblyline.ComposedSourceDocument, error),
) directCodingArtifactAdapter {
	return executableArtifactAdapter(
		id,
		directCodingArtifactValidation{
			Kind: directCodingArtifactStructural, Execute: validateStructure,
		},
		recognize,
		composeDocument...,
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

// plainTextArtifactRecognizer covers normalized plain-text leaves that have no
// richer registered parser. Unknown or non-normalized paths remain loud
// adapter-selection failures.
func plainTextArtifactRecognizer(value string) (assemblyline.TargetArtifactKind, bool) {
	normalized, err := requireExactDirectCodingPath(value)
	if err != nil || normalized != value {
		return "", false
	}
	base := path.Base(value)
	stableText := base == ".gitignore" ||
		strings.EqualFold(path.Ext(base), ".txt") && !strings.EqualFold(base, ".txt")
	if !stableText {
		return "", false
	}
	return assemblyline.TargetArtifactImplementation, true
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
