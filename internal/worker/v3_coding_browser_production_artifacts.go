package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func validateDirectCodingBrowserProductionArtifacts(root string) error {
	distRoot := filepath.Join(root, "dist")
	indexPath := filepath.Join(distRoot, "index.html")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read browser production entrypoint: %w", err)
	}
	if len(index) == 0 || !strings.Contains(string(index), `id="root"`) {
		return fmt.Errorf("browser production build lacks its non-empty root entrypoint")
	}
	css, err := directCodingBrowserBuiltCSS(distRoot)
	if err != nil {
		return err
	}
	classes, err := directCodingBrowserSourceTailwindClasses(filepath.Join(root, "src"))
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

func directCodingBrowserBuiltCSS(distRoot string) (string, error) {
	var content strings.Builder
	files := 0
	err := filepath.WalkDir(distRoot, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".css") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 {
			return fmt.Errorf("browser production CSS %q is not one non-empty regular file", candidate)
		}
		files++
		if files > 64 || info.Size() > 16*1024*1024 || content.Len() > 16*1024*1024-int(info.Size()) {
			return fmt.Errorf("browser production CSS exceeds its deterministic evidence bound")
		}
		source, err := os.ReadFile(candidate)
		if err != nil {
			return err
		}
		content.Write(source)
		content.WriteByte('\n')
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspect browser production CSS: %w", err)
	}
	if files == 0 {
		return "", fmt.Errorf("browser production build emitted no CSS artifact")
	}
	return content.String(), nil
}

func directCodingBrowserSourceTailwindClasses(sourceRoot string) ([]string, error) {
	classes := make(map[string]struct{})
	err := filepath.WalkDir(sourceRoot, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".tsx") {
			return nil
		}
		source, err := os.ReadFile(candidate)
		if err != nil {
			return err
		}
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
	})
	if err != nil {
		return nil, fmt.Errorf("extract assembled Tailwind utilities: %w", err)
	}
	ordered := make([]string, 0, len(classes))
	for className := range classes {
		ordered = append(ordered, className)
	}
	sort.Strings(ordered)
	return ordered, nil
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
