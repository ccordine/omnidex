package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func directCodingRustIdentifierChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	failedStart int,
	failedEnd int,
	failed string,
	root *treesitter.Node,
	source []byte,
	at *treesitter.Node,
	bindings map[uintptr]struct{},
	catalog directCodingRustAuthorityCatalog,
) ([]assemblyline.OpaqueModelChoice, error) {
	candidates := make([]directCodingIdentifierCandidate, 0, len(catalog.values))
	walkRustTree(root, func(node *treesitter.Node) {
		if node == nil {
			return
		}
		if _, binding := bindings[node.Id()]; !binding {
			return
		}
		if parent := node.Parent(); parent != nil && parent.Kind() == "function_item" {
			name := parent.ChildByFieldName("name")
			if name != nil && name.Id() == node.Id() {
				return
			}
		}
		if directCodingTreeBindingAvailableAt(node, at, directCodingRustScopeKind) {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: rustNodeText(source, node), role: "lexically in-scope value",
			})
		}
	})
	for name := range catalog.values {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted direct value",
		})
	}
	return directCodingRustValidatedChoices(
		input, body, failedStart, failedEnd, failed, candidates,
	)
}

func directCodingRustFunctionCandidates(
	root *treesitter.Node,
	source []byte,
	at *treesitter.Node,
	argumentCount int,
	catalog directCodingRustAuthorityCatalog,
) []directCodingIdentifierCandidate {
	candidates := make([]directCodingIdentifierCandidate, 0, len(catalog.functions)+4)
	for name, arity := range catalog.functions {
		if arity == argumentCount {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: name, role: fmt.Sprintf("permitted function with %d parameters", arity),
			})
		}
	}
	for name, arity := range map[string]int{"Err": 1, "Ok": 1, "Some": 1} {
		if arity == argumentCount {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: name, role: fmt.Sprintf("predeclared constructor with %d parameters", arity),
			})
		}
	}
	walkRustTree(root, func(node *treesitter.Node) {
		if node == nil {
			return
		}
		var name *treesitter.Node
		arity := -1
		role := ""
		switch node.Kind() {
		case "function_item":
			name = node.ChildByFieldName("name")
			arity = rustParameterCount(node)
			role = fmt.Sprintf("current function with %d parameters", arity)
		case "parameter":
			if nodeType := node.ChildByFieldName("type"); nodeType != nil &&
				nodeType.Kind() == "function_type" {
				name = rustSinglePatternIdentifier(node.ChildByFieldName("pattern"))
				arity = rustParameterCount(nodeType)
				role = fmt.Sprintf("in-scope function parameter with %d parameters", arity)
			}
		case "let_declaration":
			value := node.ChildByFieldName("value")
			nodeType := node.ChildByFieldName("type")
			if value != nil && value.Kind() == "closure_expression" {
				name = rustSinglePatternIdentifier(node.ChildByFieldName("pattern"))
				arity = rustParameterCount(value.ChildByFieldName("parameters"))
				role = fmt.Sprintf("in-scope closure with %d parameters", arity)
			} else if nodeType != nil && nodeType.Kind() == "function_type" {
				name = rustSinglePatternIdentifier(node.ChildByFieldName("pattern"))
				arity = rustParameterCount(nodeType)
				role = fmt.Sprintf("in-scope function value with %d parameters", arity)
			}
		}
		if name != nil && arity == argumentCount &&
			directCodingTreeBindingAvailableAt(name, at, directCodingRustScopeKind) {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: rustNodeText(source, name), role: role,
			})
		}
	})
	return candidates
}

func directCodingRustMacroCandidates(
	catalog directCodingRustAuthorityCatalog,
) []directCodingIdentifierCandidate {
	candidates := make([]directCodingIdentifierCandidate, 0, len(catalog.macros)+3)
	for name := range catalog.macros {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted macro",
		})
	}
	for _, name := range []string{"format", "matches", "vec"} {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "predeclared macro",
		})
	}
	return candidates
}

func directCodingRustPathRootCandidates(
	root *treesitter.Node,
	source []byte,
	catalog directCodingRustAuthorityCatalog,
) []directCodingIdentifierCandidate {
	candidates := make([]directCodingIdentifierCandidate, 0, len(catalog.pathRoots)+12)
	for name := range catalog.pathRoots {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted type or module path root",
		})
	}
	for _, name := range []string{
		"Default", "From", "Into", "Iterator", "Option", "Result", "String", "ToString", "Vec",
	} {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "predeclared type or trait path root",
		})
	}
	for _, name := range rustCurrentTypeParameterNames(root, source) {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "current generic type parameter path root",
		})
	}
	return candidates
}

func directCodingRustTypeCandidates(
	root *treesitter.Node,
	source []byte,
	catalog directCodingRustAuthorityCatalog,
) []directCodingIdentifierCandidate {
	candidates := make([]directCodingIdentifierCandidate, 0, len(catalog.types)+12)
	for name := range catalog.types {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted type",
		})
	}
	for _, name := range []string{
		"Default", "From", "Into", "Iterator", "Option", "Result", "String", "ToString", "Vec",
	} {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "predeclared type or trait",
		})
	}
	for _, name := range rustCurrentTypeParameterNames(root, source) {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "current generic type parameter",
		})
	}
	return candidates
}

func directCodingRustValidatedChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	failedStart int,
	failedEnd int,
	failed string,
	candidates []directCodingIdentifierCandidate,
) ([]assemblyline.OpaqueModelChoice, error) {
	candidates = directCodingTrialIdentifierCandidates(
		body, failedStart, failedEnd, candidates,
		func(trial string) error {
			_, err := validateDirectCodingRustFragment(input, trial)
			return err
		},
	)
	return directCodingIdentifierChoices("Rust", failed, candidates)
}

func directCodingRustCandidateNamed(
	candidates []directCodingIdentifierCandidate,
	name string,
) bool {
	for _, candidate := range candidates {
		if candidate.name == name {
			return true
		}
	}
	return false
}

func rustSinglePatternIdentifier(node *treesitter.Node) *treesitter.Node {
	if node != nil && (node.Kind() == "identifier" || node.Kind() == "shorthand_field_identifier") {
		return node
	}
	return nil
}

func rustCurrentTypeParameterNames(root *treesitter.Node, source []byte) []string {
	result := make([]string, 0)
	walkRustTree(root, func(node *treesitter.Node) {
		if node == nil || node.Kind() != "function_item" {
			return
		}
		parameters := node.ChildByFieldName("type_parameters")
		if parameters == nil {
			return
		}
		for index := uint(0); index < parameters.NamedChildCount(); index++ {
			parameter := parameters.NamedChild(index)
			var name *treesitter.Node
			switch parameter.Kind() {
			case "type_identifier":
				name = parameter
			case "constrained_type_parameter":
				name = parameter.ChildByFieldName("left")
			case "optional_type_parameter":
				name = parameter.ChildByFieldName("name")
				if name != nil && name.Kind() == "constrained_type_parameter" {
					name = name.ChildByFieldName("left")
				}
			}
			if name != nil && name.Kind() == "type_identifier" {
				result = append(result, rustNodeText(source, name))
			}
		}
	})
	return result
}

func directCodingRustScopeKind(kind string) bool {
	switch kind {
	case "block", "function_item", "closure_expression", "for_expression", "match_arm":
		return true
	default:
		return false
	}
}
