package assemblyline

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const targetTreeRootLine = "ROOT"

type rawTargetTreeNode struct {
	name      string
	path      string
	directory bool
	children  map[string]*rawTargetTreeNode
}

// ParseTargetTree is the sole raw-response grammar boundary. Models name only
// tree nodes; this parser constructs every normalized relative file path.
func ParseTargetTree(raw string) (TargetTree, error) {
	var zero TargetTree
	if raw == "" || len(raw) > maxPortableCandidateBytes || !utf8.ValidString(raw) ||
		strings.ContainsRune(raw, '\x00') {
		return zero, fmt.Errorf("raw target tree must be bounded non-empty UTF-8 text")
	}
	if strings.ContainsRune(raw, '\r') {
		return zero, fmt.Errorf("raw target tree must use LF line endings")
	}
	text := strings.TrimSuffix(raw, "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != targetTreeRootLine {
		return zero, fmt.Errorf("raw target tree must begin with the exact ROOT line")
	}
	root := &rawTargetTreeNode{directory: true, children: make(map[string]*rawTargetTreeNode)}
	stack := []*rawTargetTreeNode{root}
	directories := make([]*rawTargetTreeNode, 0)
	files := make([]string, 0)
	for lineIndex, line := range lines[1:] {
		if line == "" {
			return zero, fmt.Errorf("raw target tree line %d is blank", lineIndex+2)
		}
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		if indent == 0 || indent%2 != 0 {
			return zero, fmt.Errorf("raw target tree line %d must use two-space indentation", lineIndex+2)
		}
		depth := indent / 2
		if depth > len(stack) {
			return zero, fmt.Errorf("raw target tree line %d skips a parent depth", lineIndex+2)
		}
		if depth < len(stack) {
			stack = stack[:depth]
		}
		parent := stack[depth-1]
		if !parent.directory {
			return zero, fmt.Errorf("raw target tree line %d makes a file a parent", lineIndex+2)
		}
		kind, name, err := parseRawTargetTreeNode(line[indent:])
		if err != nil {
			return zero, fmt.Errorf("raw target tree line %d: %w", lineIndex+2, err)
		}
		if _, exists := parent.children[name]; exists {
			return zero, fmt.Errorf("raw target tree line %d duplicates or collides with one sibling", lineIndex+2)
		}
		nodePath := name
		if parent.path != "" {
			nodePath = path.Join(parent.path, name)
		}
		if err := validateTargetTreePath(nodePath); err != nil {
			return zero, fmt.Errorf("raw target tree line %d: %w", lineIndex+2, err)
		}
		node := &rawTargetTreeNode{
			name: name, path: nodePath, directory: kind == 'D',
			children: make(map[string]*rawTargetTreeNode),
		}
		parent.children[name] = node
		stack = append(stack, node)
		if node.directory {
			directories = append(directories, node)
		} else {
			files = append(files, node.path)
		}
	}
	for _, directory := range directories {
		if len(directory.children) == 0 {
			return zero, fmt.Errorf("raw target tree contains an empty directory")
		}
	}
	sort.Strings(files)
	return TargetTree{Paths: files}, nil
}

func parseRawTargetTreeNode(raw string) (byte, string, error) {
	var kind byte
	switch {
	case strings.HasPrefix(raw, "D "):
		kind = 'D'
	case strings.HasPrefix(raw, "F "):
		kind = 'F'
	default:
		return 0, "", fmt.Errorf("node must be D <single basename> or F <single basename>")
	}
	name := raw[2:]
	if err := validateTargetTreeBasename(name); err != nil {
		return 0, "", err
	}
	return kind, name, nil
}

func validateTargetTreeBasename(name string) error {
	if name == "" || name != strings.TrimSpace(name) || name == "." || name == ".." ||
		strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, "~") ||
		len(name) > maxTargetTreePathBytes || !utf8.ValidString(name) {
		return fmt.Errorf("node name must be one safe basename")
	}
	if len(name) >= 2 && isASCIIAlpha(name[0]) && name[1] == ':' {
		return fmt.Errorf("node name must not be an absolute drive identity")
	}
	for _, value := range name {
		if unicode.IsControl(value) {
			return fmt.Errorf("node name must not contain control characters")
		}
	}
	return nil
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

// RenderTargetTree renders code-owned path facts in the same canonical tree
// shape used by the raw response grammar. Directories precede files and each
// group is ordered by basename.
func RenderTargetTree(paths []string) (string, error) {
	if paths == nil {
		return "", fmt.Errorf("target tree paths must be a non-nil array")
	}
	if err := validateTargetTreePaths("render path", paths); err != nil {
		return "", err
	}
	root := &rawTargetTreeNode{directory: true, children: make(map[string]*rawTargetTreeNode)}
	for _, artifactPath := range paths {
		parts := strings.Split(artifactPath, "/")
		parent := root
		for index, name := range parts {
			last := index == len(parts)-1
			existing, exists := parent.children[name]
			if exists {
				if last || !existing.directory {
					return "", fmt.Errorf("target tree render path %q collides with another node", artifactPath)
				}
				parent = existing
				continue
			}
			node := &rawTargetTreeNode{name: name, directory: !last, children: make(map[string]*rawTargetTreeNode)}
			parent.children[name] = node
			parent = node
		}
	}
	lines := []string{targetTreeRootLine}
	renderRawTargetTreeChildren(root, 1, &lines)
	return strings.Join(lines, "\n"), nil
}

func renderRawTargetTreeChildren(parent *rawTargetTreeNode, depth int, lines *[]string) {
	children := make([]*rawTargetTreeNode, 0, len(parent.children))
	for _, child := range parent.children {
		children = append(children, child)
	}
	sort.Slice(children, func(left, right int) bool {
		if children[left].directory != children[right].directory {
			return children[left].directory
		}
		return children[left].name < children[right].name
	})
	for _, child := range children {
		kind := "F"
		if child.directory {
			kind = "D"
		}
		*lines = append(*lines, strings.Repeat("  ", depth)+kind+" "+child.name)
		if child.directory {
			renderRawTargetTreeChildren(child, depth+1, lines)
		}
	}
}
