package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func (flow directCodingBrowserOutputDataflow) projectBuiltinOutputCall(
	call *treesitter.Node,
) ([]*treesitter.Node, bool) {
	if call == nil || call.Kind() != "call_expression" {
		return nil, false
	}
	callee := directCodingBrowserUnwrapRuntimeExpression(call.ChildByFieldName("function"))
	arguments, valid := directCodingBrowserOutputCallArguments(call)
	if !valid || callee == nil {
		return nil, false
	}
	if callee.Kind() == "identifier" && flow.resolve(flow.text(callee), callee) == nil {
		switch flow.text(callee) {
		case "String", "Number", "Boolean", "BigInt", "parseFloat", "isNaN",
			"isFinite", "encodeURI", "encodeURIComponent", "decodeURI",
			"decodeURIComponent":
			return directCodingBrowserOutputConsumedArguments(arguments, 1), true
		case "parseInt":
			return directCodingBrowserOutputConsumedArguments(arguments, 2), true
		}
		return nil, false
	}
	if callee.Kind() != "member_expression" && callee.Kind() != "subscript_expression" {
		return nil, false
	}
	name, resolved := flow.outputPropertyName(callee)
	if !resolved {
		return nil, false
	}
	receiver := directCodingBrowserUnwrapRuntimeExpression(callee.ChildByFieldName("object"))
	if receiver == nil {
		return nil, false
	}
	if receiver.Kind() == "identifier" && flow.resolve(flow.text(receiver), receiver) == nil {
		if children, known := directCodingBrowserOutputStaticBuiltin(
			flow.text(receiver), name, arguments,
		); known {
			return children, true
		}
	}
	container, structural := flow.resolveOutputContainer(receiver, make(map[uintptr]struct{}))
	if structural && (container == nil || container.Kind() != "array") {
		return nil, false
	}
	limit, all, known := directCodingBrowserOutputInstanceBuiltin(name)
	if !known {
		return nil, false
	}
	children := []*treesitter.Node{receiver}
	if all {
		return append(children, arguments...), true
	}
	return append(children, directCodingBrowserOutputConsumedArguments(arguments, limit)...), true
}

func directCodingBrowserOutputStaticBuiltin(
	receiver string,
	name string,
	arguments []*treesitter.Node,
) ([]*treesitter.Node, bool) {
	switch receiver {
	case "Math":
		switch name {
		case "max", "min", "hypot":
			return arguments, true
		case "atan2", "imul", "pow":
			return directCodingBrowserOutputConsumedArguments(arguments, 2), true
		case "abs", "acos", "acosh", "asin", "asinh", "atan", "atanh", "cbrt",
			"ceil", "clz32", "cos", "cosh", "exp", "expm1", "floor", "fround",
			"log", "log10", "log1p", "log2", "round", "sign", "sin", "sinh",
			"sqrt", "tan", "tanh", "trunc":
			return directCodingBrowserOutputConsumedArguments(arguments, 1), true
		}
	case "Number":
		switch name {
		case "isFinite", "isInteger", "isNaN", "isSafeInteger", "parseFloat":
			return directCodingBrowserOutputConsumedArguments(arguments, 1), true
		case "parseInt":
			return directCodingBrowserOutputConsumedArguments(arguments, 2), true
		}
	case "JSON":
		switch name {
		case "parse":
			return directCodingBrowserOutputConsumedArguments(arguments, 2), true
		case "stringify":
			return directCodingBrowserOutputConsumedArguments(arguments, 3), true
		}
	case "Array":
		switch name {
		case "from":
			return directCodingBrowserOutputConsumedArguments(arguments, 3), true
		case "isArray":
			return directCodingBrowserOutputConsumedArguments(arguments, 1), true
		case "of":
			return arguments, true
		}
	case "Object":
		switch name {
		case "entries", "keys", "values":
			return directCodingBrowserOutputConsumedArguments(arguments, 1), true
		case "fromEntries":
			return directCodingBrowserOutputConsumedArguments(arguments, 1), true
		case "is":
			return directCodingBrowserOutputConsumedArguments(arguments, 2), true
		}
	}
	return nil, false
}

func directCodingBrowserOutputInstanceBuiltin(name string) (int, bool, bool) {
	switch name {
	case "concat":
		return 0, true, true
	case "at", "charAt", "charCodeAt", "codePointAt", "repeat", "toFixed",
		"toPrecision":
		return 1, false, true
	case "endsWith", "includes", "indexOf", "lastIndexOf", "padEnd", "padStart",
		"slice", "startsWith", "substring", "substr":
		return 2, false, true
	case "replace", "replaceAll", "split":
		return 2, false, true
	case "join", "toExponential":
		return 1, false, true
	case "toString", "trim", "trimEnd", "trimStart", "toLowerCase", "toUpperCase",
		"valueOf":
		return 0, false, true
	default:
		return 0, false, false
	}
}

func directCodingBrowserOutputConsumedArguments(
	arguments []*treesitter.Node,
	limit int,
) []*treesitter.Node {
	if limit >= len(arguments) {
		return arguments
	}
	if limit <= 0 {
		return nil
	}
	return arguments[:limit]
}
