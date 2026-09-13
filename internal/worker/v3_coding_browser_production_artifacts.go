package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/experiment"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func validateDirectCodingBrowserProductionArtifacts(assembly directCodingAssembly, artifacts []experiment.File) error {
	var index []byte
	for _, file := range artifacts {
		if file.Path == "dist/index.html" {
			index = file.Content
		}
	}
	if len(index) == 0 || !strings.Contains(string(index), `id="root"`) {
		return fmt.Errorf("browser production build lacks its non-empty root entrypoint")
	}
	css, err := directCodingBrowserBuiltCSS(artifacts)
	if err != nil {
		return err
	}
	classes, err := directCodingBrowserSourceTailwindClasses(assembly.Files)
	if err != nil {
		return err
	}
	if len(classes) == 0 {
		return fmt.Errorf("browser assembly contains no statically provable Tailwind utilities")
	}
	for _, className := range classes {
		selector := "." + directCodingTailwindSelectorEscape(className)
		if !strings.Contains(css, selector) {
			return fmt.Errorf(
				"browser production CSS omitted assembled Tailwind utility %q", className,
			)
		}
	}
	return nil
}

func directCodingBrowserBuiltCSS(artifacts []experiment.File) (string, error) {
	var content strings.Builder
	files := 0
	for _, file := range artifacts {
		if !strings.HasPrefix(file.Path, "dist/") || !strings.HasSuffix(strings.ToLower(file.Path), ".css") {
			continue
		}
		if file.Mode == 0 || file.Mode&^uint32(0o777) != 0 || len(file.Content) == 0 {
			return "", fmt.Errorf("browser production CSS %q is not one non-empty regular file", file.Path)
		}
		files++
		if files > 64 || len(file.Content) > 16*1024*1024 || content.Len() > 16*1024*1024-len(file.Content) {
			return "", fmt.Errorf("browser production CSS exceeds its deterministic evidence bound")
		}
		content.Write(file.Content)
		content.WriteByte('\n')
	}
	if files == 0 {
		return "", fmt.Errorf("browser production build emitted no CSS artifact")
	}
	return content.String(), nil
}

func directCodingBrowserSourceTailwindClasses(files []directCodingFileTask) ([]string, error) {
	classes := make(map[string]struct{})
	for _, file := range files {
		if !strings.HasPrefix(file.Path, "src/") || !strings.HasSuffix(strings.ToLower(file.Path), ".tsx") {
			continue
		}
		if err := extractDirectCodingBrowserTailwindClasses(file.Path, file.Content, classes); err != nil {
			return nil, fmt.Errorf("extract assembled Tailwind utilities: %w", err)
		}
	}
	ordered := make([]string, 0, len(classes))
	for className := range classes {
		ordered = append(ordered, className)
	}
	sort.Strings(ordered)
	return ordered, nil
}

func extractDirectCodingBrowserTailwindClasses(candidate string, source []byte, classes map[string]struct{}) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(typescript.LanguageTSX())); err != nil {
		return err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("Tailwind source parser returned no tree for %s", candidate)
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return fmt.Errorf("Tailwind source %s is not valid TSX", candidate)
	}
	extractor := directCodingBrowserPublicSurfaceExtractor{source: source}
	var inspect func(*treesitter.Node) error
	inspect = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		if node.Kind() == "jsx_attribute" && node.NamedChildCount() == 2 {
			name := node.NamedChild(0)
			if name != nil && extractor.nodeText(name) == "className" {
				attribute, err := extractor.exactAttribute(node, "className")
				if err != nil {
					return err
				}
				for _, className := range strings.Fields(attribute.literal) {
					if err := validateDirectCodingBrowserSafeTailwindClass(className); err != nil {
						return err
					}
					classes[className] = struct{}{}
				}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if err := inspect(node.NamedChild(index)); err != nil {
				return err
			}
		}
		return nil
	}
	return inspect(root)
}

func directCodingTailwindSelectorEscape(className string) string {
	var escaped strings.Builder
	for _, character := range className {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' {
			escaped.WriteRune(character)
			continue
		}
		escaped.WriteByte('\\')
		escaped.WriteRune(character)
	}
	return escaped.String()
}
