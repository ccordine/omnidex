package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/pelletier/go-toml/v2"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	htmlgrammar "github.com/tree-sitter/tree-sitter-html/bindings/go"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
	javascriptgrammar "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	phpgrammar "github.com/tree-sitter/tree-sitter-php/bindings/go"
	rustgrammar "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

func validateDirectCodingArtifactSource(
	adapter directCodingArtifactAdapter,
	artifactPath string,
	source []byte,
) error {
	if adapter.ID == "" || adapter.Validation.Execute == nil {
		return fmt.Errorf("artifact adapter %q has no executable source validator", adapter.ID)
	}
	if err := validateArtifactText(artifactPath, source); err != nil {
		return fmt.Errorf("%s adapter source: %w", adapter.ID, err)
	}
	if err := adapter.Validation.Execute(artifactPath, source); err != nil {
		return fmt.Errorf("%s adapter rejected %s: %w", adapter.ID, artifactPath, err)
	}
	return nil
}

func validateArtifactText(artifactPath string, source []byte) error {
	if strings.TrimSpace(artifactPath) == "" {
		return fmt.Errorf("artifact path is required")
	}
	if len(source) == 0 {
		return fmt.Errorf("artifact %s is empty", artifactPath)
	}
	if !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return fmt.Errorf("artifact %s must be valid UTF-8 without NUL bytes", artifactPath)
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

func validatePHPArtifactSource(_ string, source []byte) error {
	return validateTreeSitterArtifact("PHP", phpgrammar.LanguagePHP(), source)
}

func validatePHPExecutableArtifactSource(artifactPath string, source []byte) error {
	if path.Base(artifactPath) != "artisan" ||
		!bytes.HasPrefix(source, []byte("#!/usr/bin/env php\n<?php\n")) {
		return fmt.Errorf("PHP executable requires the exact artisan launcher path and shebang")
	}
	return validatePHPArtifactSource(artifactPath, source)
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

func validateYAMLArtifactSource(_ string, source []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	documents := 0
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("parse YAML document %d: %w", documents+1, err)
		}
		documents++
	}
	if documents == 0 {
		return fmt.Errorf("YAML source contains no document")
	}
	return nil
}

func validateCSSArtifactSource(_ string, source []byte) error {
	return validateBalancedArtifact("CSS", source, true, false)
}

func validateNginxArtifactSource(_ string, source []byte) error {
	if err := validateBalancedArtifact("NGINX", source, false, true); err != nil {
		return err
	}
	return validateNginxStatements(source)
}

func validateDockerArtifactSource(artifactPath string, source []byte) error {
	base := strings.ToLower(path.Base(artifactPath))
	if strings.HasPrefix(base, "docker-compose") {
		return validateYAMLArtifactSource(artifactPath, source)
	}
	return validateDockerfileStatements(source)
}

func validateEnvironmentArtifactSource(_ string, source []byte) error {
	for index, raw := range strings.Split(string(source), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		separator := strings.IndexByte(line, '=')
		if separator <= 0 || !validEnvironmentKey(line[:separator]) {
			return fmt.Errorf("invalid environment assignment at line %d", index+1)
		}
	}
	return nil
}

func validEnvironmentKey(value string) bool {
	for index, char := range value {
		if index == 0 && char != '_' && !unicode.IsLetter(char) {
			return false
		}
		if index > 0 && char != '_' && !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
	}
	return value != ""
}

func validatePlainTextArtifactSource(_ string, _ []byte) error {
	return nil
}

func validateTypeScriptBrowserAssembly(assembly directCodingAssembly) error {
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
	}
	index, exists := files["index.html"]
	if !exists {
		return nil
	}
	manifest, manifestExists := files["package.json"]
	lock, lockExists := files["package-lock.json"]
	if !manifestExists || !lockExists {
		return fmt.Errorf("TypeScript browser assembly requires package.json and package-lock.json")
	}
	if err := validatePinnedNPMLockForManifest(manifest, lock); err != nil {
		return fmt.Errorf("TypeScript browser dependency authority: %w", err)
	}
	if !strings.Contains(index, `id="root"`) || !strings.Contains(index, `/src/main.tsx`) {
		return fmt.Errorf("HTML entrypoint lacks its required root or TypeScript module")
	}
	main, exists := files["src/main.tsx"]
	if !exists {
		return fmt.Errorf("HTML entrypoint references absent artifact src/main.tsx")
	}
	if !strings.Contains(main, `from './App'`) || !strings.Contains(main, `import './styles.css'`) {
		return fmt.Errorf("TypeScript entrypoint lacks its required application or stylesheet relation")
	}
	for _, required := range []string{"src/App.tsx", "src/styles.css"} {
		if _, exists := files[required]; !exists {
			return fmt.Errorf("TypeScript entrypoint references absent artifact %s", required)
		}
	}
	if err := validateTypeScriptBrowserTailwindAuthority(files); err != nil {
		return err
	}
	return nil
}
