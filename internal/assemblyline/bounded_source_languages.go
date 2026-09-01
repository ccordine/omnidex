package assemblyline

import (
	"fmt"
	"path"
	"unsafe"

	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

type boundedSourceLanguage struct {
	id                   string
	display              string
	declaration          string
	documentLanguage     func() unsafe.Pointer
	fragmentLanguage     func() unsafe.Pointer
	pathAllowed          func(string) bool
	declarationKinds     map[string]struct{}
	allowCodeOwnedExport bool
}

func javaScriptSourceLanguage() boundedSourceLanguage {
	return boundedSourceLanguage{
		id: "javascript", display: "JavaScript", declaration: "function declaration",
		documentLanguage: javascript.Language, fragmentLanguage: javascript.Language,
		pathAllowed:          sourceExtensionAllowed(".js", ".jsx", ".mjs", ".cjs"),
		declarationKinds:     sourceNodeKinds("function_declaration"),
		allowCodeOwnedExport: true,
	}
}

func javaSourceLanguage() boundedSourceLanguage {
	return boundedSourceLanguage{
		id: "java", display: "Java", declaration: "method declaration",
		documentLanguage: java.Language, fragmentLanguage: java.Language,
		pathAllowed:      sourceExtensionAllowed(".java"),
		declarationKinds: sourceNodeKinds("method_declaration"),
	}
}

func rustSourceLanguage() boundedSourceLanguage {
	return boundedSourceLanguage{
		id: "rust", display: "Rust", declaration: "function declaration",
		documentLanguage: rust.Language, fragmentLanguage: rust.Language,
		pathAllowed:      sourceExtensionAllowed(".rs"),
		declarationKinds: sourceNodeKinds("function_item"),
	}
}

func boundedSourceLanguageByID(id string) (boundedSourceLanguage, error) {
	for _, language := range []boundedSourceLanguage{
		javaScriptSourceLanguage(), javaSourceLanguage(), rustSourceLanguage(),
	} {
		if language.id == id {
			return language, nil
		}
	}
	return boundedSourceLanguage{}, fmt.Errorf("bounded source declaration does not support language %q", id)
}

func sourceExtensionAllowed(extensions ...string) func(string) bool {
	return func(value string) bool {
		extension := path.Ext(value)
		for _, allowed := range extensions {
			if extension == allowed {
				return true
			}
		}
		return false
	}
}

func sourceNodeKinds(values ...string) map[string]struct{} {
	kinds := make(map[string]struct{}, len(values))
	for _, value := range values {
		kinds[value] = struct{}{}
	}
	return kinds
}
