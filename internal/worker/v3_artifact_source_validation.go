package worker

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"path"
	"strings"
	"unsafe"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/pelletier/go-toml/v2"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	htmlgrammar "github.com/tree-sitter/tree-sitter-html/bindings/go"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
	javascriptgrammar "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	rustgrammar "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	"golang.org/x/mod/modfile"
)

func validateDirectCodingArtifactSource(
	adapter directCodingArtifactAdapter,
	artifactPath string,
	source []byte,
) error {
	if adapter.ID == "" || adapter.Validation.Execute == nil {
		return fmt.Errorf("artifact adapter %q has no executable source validator", adapter.ID)
	}
	if err := adapter.Validation.Execute(artifactPath, source); err != nil {
		return fmt.Errorf("%s adapter rejected %s: %w", adapter.ID, artifactPath, err)
	}
	return nil
}

func validateTypeScriptReactArtifactSource(_ string, source []byte) error {
	return assemblyline.ValidateTypeScriptSource(string(source), true)
}

func validateTypeScriptArtifactSource(_ string, source []byte) error {
	return assemblyline.ValidateTypeScriptSource(string(source), false)
}

func validateGoArtifactSource(artifactPath string, source []byte) error {
	if _, err := parser.ParseFile(token.NewFileSet(), artifactPath, source, parser.AllErrors); err != nil {
		return fmt.Errorf("parse Go source: %w", err)
	}
	return nil
}

func validateGoModuleArtifactSource(artifactPath string, source []byte) error {
	module, err := modfile.Parse(artifactPath, source, nil)
	if err != nil {
		return fmt.Errorf("parse Go module manifest: %w", err)
	}
	if module.Module == nil || module.Module.Mod.Path == "" {
		return fmt.Errorf("Go module manifest requires one module directive")
	}
	if module.Go == nil || module.Go.Version == "" {
		return fmt.Errorf("Go module manifest requires one go directive")
	}
	return nil
}

func validateCargoTOMLArtifactSource(artifactPath string, source []byte) error {
	var document map[string]any
	if err := toml.Unmarshal(source, &document); err != nil {
		return fmt.Errorf("parse Cargo TOML: %w", err)
	}
	switch path.Base(artifactPath) {
	case "Cargo.toml":
		packageValue, exists := document["package"]
		packageTable, valid := packageValue.(map[string]any)
		if !exists || !valid {
			return fmt.Errorf("Cargo manifest requires one package table")
		}
		for _, field := range []string{"name", "version", "edition"} {
			value, valid := packageTable[field].(string)
			if !valid || strings.TrimSpace(value) == "" {
				return fmt.Errorf("Cargo manifest package requires %s", field)
			}
		}
	case "Cargo.lock":
		if _, exists := document["version"]; !exists {
			return fmt.Errorf("Cargo lockfile requires a format version")
		}
		if _, exists := document["package"]; !exists {
			return fmt.Errorf("Cargo lockfile requires at least one package")
		}
	default:
		return fmt.Errorf("Cargo TOML validator received unsupported artifact %s", artifactPath)
	}
	return nil
}

func validateJavaScriptArtifactSource(_ string, source []byte) error {
	return validateTreeSitterArtifact("JavaScript", javascriptgrammar.Language(), source)
}

func validateHTMLArtifactSource(_ string, source []byte) error {
	return validateTreeSitterArtifact("HTML", htmlgrammar.Language(), source)
}

func validateJavaArtifactSource(_ string, source []byte) error {
	return validateTreeSitterArtifact("Java", javagrammar.Language(), source)
}

func validateRustArtifactSource(_ string, source []byte) error {
	return validateTreeSitterArtifact("Rust", rustgrammar.Language(), source)
}

func validateTreeSitterArtifact(label string, language unsafe.Pointer, source []byte) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(language)); err != nil {
		return fmt.Errorf("configure %s parser: %w", label, err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("%s parser returned no syntax tree", label)
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return fmt.Errorf("%s parser returned no root node", label)
	}
	if !root.HasError() {
		return nil
	}
	failure := firstTreeSitterFailure(root)
	if failure == nil {
		return fmt.Errorf("%s parser reported an unlocated syntax failure", label)
	}
	point := failure.StartPosition()
	return fmt.Errorf(
		"%s syntax failure %s at line %d column %d",
		label, failure.Kind(), point.Row+1, point.Column+1,
	)
}

func firstTreeSitterFailure(node *treesitter.Node) *treesitter.Node {
	if node == nil || !node.HasError() {
		return nil
	}
	if node.IsError() || node.IsMissing() {
		return node
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if failure := firstTreeSitterFailure(node.Child(index)); failure != nil {
			return failure
		}
	}
	return nil
}

func validateJSONArtifactSource(_ string, source []byte) error {
	if !json.Valid(source) {
		return fmt.Errorf("invalid JSON document")
	}
	return nil
}

func validateCSSArtifactSource(_ string, source []byte) error {
	return validateBalancedArtifact("CSS", source, true, false)
}

func validatePlainTextArtifactSource(_ string, source []byte) error {
	return assemblyline.ValidateTextFragment(string(source))
}
