package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func validateRustFragmentMacro(
	source []byte,
	node *treesitter.Node,
	permitted map[string]struct{},
) error {
	macro := node.ChildByFieldName("macro")
	if macro == nil {
		return fmt.Errorf("Rust fragment macro has no code-owned identity")
	}
	name := rustNodeText(source, macro)
	if rustFragmentForbiddenSymbol(name) || rustFragmentForbiddenMacro(name) {
		return fmt.Errorf("Rust fragment uses forbidden macro authority %s", name)
	}
	if _, allowed := permitted[name]; allowed || rustFragmentPreludeMacro(name) {
		return nil
	}
	return fmt.Errorf("Rust fragment macro %s is outside declared authority", name)
}

func validateRustFragmentCall(
	source []byte,
	node *treesitter.Node,
	locals map[string]struct{},
	permitted map[string]struct{},
) error {
	callable := node.ChildByFieldName("function")
	if callable == nil {
		return fmt.Errorf("Rust fragment call has no callable authority")
	}
	return validateRustCallableAuthority(source, callable, locals, permitted)
}

func validateRustCallableAuthority(
	source []byte,
	callable *treesitter.Node,
	locals map[string]struct{},
	permitted map[string]struct{},
) error {
	switch callable.Kind() {
	case "identifier":
		name := rustNodeText(source, callable)
		if rustFragmentForbiddenSymbol(name) {
			return fmt.Errorf("Rust fragment calls forbidden environment authority %s", name)
		}
		if !rustFragmentSymbolAllowed(name, locals, permitted) {
			return fmt.Errorf("Rust fragment callable %s is outside declared authority", name)
		}
	case "scoped_identifier", "scoped_type_identifier":
		return validateRustFragmentPath(rustNodeText(source, callable), locals, permitted)
	case "field_expression":
		return nil
	case "generic_function":
		function := callable.ChildByFieldName("function")
		if function == nil {
			return fmt.Errorf("Rust generic call has no callable authority")
		}
		return validateRustCallableAuthority(source, function, locals, permitted)
	default:
		return fmt.Errorf(
			"Rust fragment uses unsupported indirect callable authority %s", callable.Kind(),
		)
	}
	return nil
}

func validateRustFragmentPath(
	value string,
	locals map[string]struct{},
	permitted map[string]struct{},
) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "<>{}()[]") {
		return fmt.Errorf("Rust fragment path %q is not one bounded symbol path", value)
	}
	parts := strings.Split(strings.TrimPrefix(value, "::"), "::")
	if len(parts) < 2 || parts[0] == "" {
		return fmt.Errorf("Rust fragment path %q is invalid", value)
	}
	for _, part := range parts {
		if rustFragmentForbiddenSymbol(part) {
			return fmt.Errorf("Rust fragment path %q uses forbidden environment authority %s", value, part)
		}
	}
	if !rustFragmentSymbolAllowed(parts[0], locals, permitted) {
		return fmt.Errorf("Rust fragment path root %s is outside declared authority", parts[0])
	}
	return nil
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
		"From", "Into", "Iterator", "ToString":
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
