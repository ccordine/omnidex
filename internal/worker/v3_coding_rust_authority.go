package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingRustAuthorityFailure struct {
	node       *treesitter.Node
	question   string
	candidates []directCodingIdentifierCandidate
	cause      error
}

func validateRustFragmentMacro(
	source []byte,
	node *treesitter.Node,
	root *treesitter.Node,
	catalog directCodingRustAuthorityCatalog,
) *directCodingRustAuthorityFailure {
	macro := node.ChildByFieldName("macro")
	if macro == nil {
		return &directCodingRustAuthorityFailure{
			node:  node,
			cause: fmt.Errorf("Rust fragment macro has no code-owned identity"),
		}
	}
	if macro.Kind() == "scoped_identifier" {
		if failure := validateRustFragmentPath(source, macro, root, catalog); failure != nil {
			return failure
		}
	}
	token := rustPathTerminalToken(macro)
	if token == nil {
		return &directCodingRustAuthorityFailure{
			node:  macro,
			cause: fmt.Errorf("Rust fragment macro has no exact identity token"),
		}
	}
	name := rustNodeText(source, token)
	candidates := directCodingRustMacroCandidates(catalog)
	if !rustFragmentForbiddenSymbol(name) && !rustFragmentForbiddenMacro(name) &&
		directCodingRustCandidateNamed(candidates, name) {
		return nil
	}
	// A direct macro token may be replaced by another mechanically known
	// macro. A module-qualified token has no proven association with those
	// direct macros, so it deliberately has no inferred replacement set.
	if macro.Kind() != "identifier" {
		candidates = nil
	}
	return &directCodingRustAuthorityFailure{
		node:       token,
		question:   "Which available macro provides the required expansion at this token?",
		candidates: candidates,
		cause:      fmt.Errorf("Rust fragment macro %s is outside declared macro authority", name),
	}
}

func validateRustFragmentCall(
	source []byte,
	node *treesitter.Node,
	root *treesitter.Node,
	catalog directCodingRustAuthorityCatalog,
) *directCodingRustAuthorityFailure {
	callable := node.ChildByFieldName("function")
	if callable == nil {
		return &directCodingRustAuthorityFailure{
			node:  node,
			cause: fmt.Errorf("Rust fragment call has no callable authority"),
		}
	}
	arguments := node.ChildByFieldName("arguments")
	argumentCount := 0
	if arguments != nil {
		argumentCount = int(arguments.NamedChildCount())
	}
	return validateRustCallableAuthority(
		source, callable, root, argumentCount, catalog,
	)
}

func validateRustCallableAuthority(
	source []byte,
	callable *treesitter.Node,
	root *treesitter.Node,
	argumentCount int,
	catalog directCodingRustAuthorityCatalog,
) *directCodingRustAuthorityFailure {
	candidates := directCodingRustFunctionCandidates(
		root, source, callable, argumentCount, catalog,
	)
	switch callable.Kind() {
	case "identifier":
		name := rustNodeText(source, callable)
		if !rustFragmentForbiddenSymbol(name) &&
			directCodingRustCandidateNamed(candidates, name) {
			return nil
		}
		return &directCodingRustAuthorityFailure{
			node:       callable,
			question:   "Which available function provides the required result at this call?",
			candidates: candidates,
			cause:      fmt.Errorf("Rust fragment callable %s is outside declared function authority", name),
		}
	case "scoped_identifier", "scoped_type_identifier":
		return validateRustFragmentPath(source, callable, root, catalog)
	case "field_expression":
		return nil
	case "generic_function":
		function := callable.ChildByFieldName("function")
		if function != nil {
			return validateRustCallableAuthority(
				source, function, root, argumentCount, catalog,
			)
		}
	}
	return &directCodingRustAuthorityFailure{
		node:       callable,
		question:   "Which available function provides the required result at this call?",
		candidates: candidates,
		cause: fmt.Errorf(
			"Rust fragment uses unsupported callable authority %s", callable.Kind(),
		),
	}
}

func validateRustFragmentPath(
	source []byte,
	node *treesitter.Node,
	root *treesitter.Node,
	catalog directCodingRustAuthorityCatalog,
) *directCodingRustAuthorityFailure {
	components := rustPathComponentNodes(node)
	if len(components) < 2 {
		return &directCodingRustAuthorityFailure{
			node:  node,
			cause: fmt.Errorf("Rust fragment path %q has no exact root and suffix", rustNodeText(source, node)),
		}
	}
	rootCandidates := directCodingRustPathRootCandidates(root, source, catalog)
	for index, component := range components {
		name := rustNodeText(source, component)
		if index == 0 {
			if !rustFragmentForbiddenSymbol(name) &&
				directCodingRustCandidateNamed(rootCandidates, name) {
				continue
			}
			return &directCodingRustAuthorityFailure{
				node:       component,
				question:   "Which available path root provides the required associated item?",
				candidates: rootCandidates,
				cause:      fmt.Errorf("Rust fragment path root %s is outside declared authority", name),
			}
		}
		if rustFragmentForbiddenSymbol(name) {
			return &directCodingRustAuthorityFailure{
				node:     component,
				question: "Which available path component has the required meaning here?",
				cause:    fmt.Errorf("Rust fragment path component %s uses forbidden authority", name),
			}
		}
	}
	return nil
}

func rustPathComponentNodes(node *treesitter.Node) []*treesitter.Node {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "scoped_identifier", "scoped_type_identifier":
		components := rustPathComponentNodes(node.ChildByFieldName("path"))
		if name := rustPathTerminalToken(node.ChildByFieldName("name")); name != nil {
			components = append(components, name)
		}
		return components
	case "identifier", "type_identifier", "crate", "self", "super":
		return []*treesitter.Node{node}
	default:
		var result []*treesitter.Node
		for index := uint(0); index < node.NamedChildCount(); index++ {
			result = append(result, rustPathComponentNodes(node.NamedChild(index))...)
		}
		return result
	}
}

func rustPathTerminalToken(node *treesitter.Node) *treesitter.Node {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "identifier", "type_identifier", "crate", "self", "super":
		return node
	case "scoped_identifier", "scoped_type_identifier":
		return rustPathTerminalToken(node.ChildByFieldName("name"))
	default:
		for index := node.NamedChildCount(); index > 0; index-- {
			if result := rustPathTerminalToken(node.NamedChild(index - 1)); result != nil {
				return result
			}
		}
		return nil
	}
}

func rustFragmentSymbolAllowed(
	name string,
	locals map[string]struct{},
	permitted map[string]struct{},
) bool {
	if rustFragmentForbiddenSymbol(name) {
		return false
	}
	if _, exists := locals[name]; exists {
		return true
	}
	if _, exists := permitted[name]; exists {
		return true
	}
	return rustFragmentPreludeSymbol(name) || rustFragmentPreludeMacro(name)
}

func rustIdentifierBelongsToPath(node, parent *treesitter.Node) bool {
	if node == nil || parent == nil {
		return false
	}
	switch parent.Kind() {
	case "scoped_identifier", "scoped_type_identifier":
		return true
	case "field_expression", "field_initializer":
		field := parent.ChildByFieldName("field")
		if field == nil {
			field = parent.ChildByFieldName("name")
		}
		return field != nil && field.Id() == node.Id()
	default:
		return false
	}
}

func rustTypeIdentifierBelongsToPath(node, parent *treesitter.Node) bool {
	if node == nil || parent == nil {
		return false
	}
	for current := parent; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "scoped_identifier", "scoped_type_identifier":
			return true
		case "function_item", "block", "let_declaration", "parameter":
			return false
		}
	}
	return false
}

// Rust macro arguments are token trees rather than expression ASTs. A token
// immediately following '.' is mechanically a member/method name, not a free
// callable or path root. The receiver remains independently inspected.
func rustIdentifierIsMemberToken(source []byte, node *treesitter.Node) bool {
	if node == nil || node.StartByte() == 0 || int(node.StartByte()) > len(source) {
		return false
	}
	for index := int(node.StartByte()) - 1; index >= 0; index-- {
		switch source[index] {
		case ' ', '\t', '\r', '\n':
			continue
		case '.':
			return true
		default:
			return false
		}
	}
	return false
}

func rustFragmentPreludeSymbol(name string) bool {
	switch name {
	case "Option", "Result", "Some", "None", "Ok", "Err", "Default",
		"From", "Into", "Iterator", "String", "ToString", "Vec":
		return true
	default:
		return false
	}
}

func rustFragmentPreludeMacro(name string) bool {
	switch name {
	case "format", "vec", "matches":
		return true
	default:
		return false
	}
}

func rustFragmentForbiddenMacro(name string) bool {
	switch name {
	case "include", "include_bytes", "include_str", "env", "option_env",
		"print", "println", "eprint", "eprintln", "dbg", "panic", "todo",
		"unimplemented", "unreachable", "asm", "global_asm":
		return true
	default:
		return false
	}
}

func rustFragmentForbiddenSymbol(name string) bool {
	name = strings.TrimSpace(strings.TrimSuffix(name, "!"))
	switch name {
	case "std", "core", "alloc", "crate", "self", "super",
		"fs", "env", "process", "net", "io", "path", "thread", "time", "sync",
		"File", "OpenOptions", "Command", "Stdio", "Path", "PathBuf",
		"TcpStream", "TcpListener", "UdpSocket", "UnixStream", "UnixListener",
		"include", "include_bytes", "include_str", "option_env",
		"print", "println", "eprint", "eprintln", "dbg", "panic", "todo",
		"unimplemented", "unreachable", "asm", "global_asm":
		return true
	default:
		return false
	}
}

func rustSimpleIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if index == 0 {
			if char != '_' && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
				return false
			}
			continue
		}
		if char != '_' && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') {
			return false
		}
	}
	return true
}
