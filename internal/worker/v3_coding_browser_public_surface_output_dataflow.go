package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserOutputDefinitionKind uint8

const (
	directCodingBrowserOutputOpaque directCodingBrowserOutputDefinitionKind = iota
	directCodingBrowserOutputRootState
	directCodingBrowserOutputAlias
	directCodingBrowserOutputLocalState
	directCodingBrowserOutputSetter
	directCodingBrowserOutputFunction
)

type directCodingBrowserOutputDefinition struct {
	id         uintptr
	nodeID     uintptr
	kind       directCodingBrowserOutputDefinitionKind
	value      *treesitter.Node
	setterID   uintptr
	available  uint
	scopeStart uint
	scopeEnd   uint
}

type directCodingBrowserOutputDataflow struct {
	source        []byte
	root          *treesitter.Node
	definitions   map[string][]*directCodingBrowserOutputDefinition
	eventBindings directCodingBrowserEventBindings
	setterCalls   map[uintptr][]directCodingBrowserSetterCall
	calledSetters map[uintptr]struct{}
}

func newDirectCodingBrowserOutputDataflow(
	root *treesitter.Node,
	render *treesitter.Node,
	source []byte,
) (directCodingBrowserOutputDataflow, error) {
	function := directCodingBrowserNearestContainingFunction(render)
	if root == nil || function == nil || function.Kind() != "function_declaration" {
		return directCodingBrowserOutputDataflow{}, fmt.Errorf(
			"browser public output dataflow requires one render function",
		)
	}
	flow := directCodingBrowserOutputDataflow{
		source: source, root: function,
		definitions:   make(map[string][]*directCodingBrowserOutputDefinition),
		setterCalls:   make(map[uintptr][]directCodingBrowserSetterCall),
		calledSetters: make(map[uintptr]struct{}),
	}
	flow.collectFunction(function, true)
	eventBindings, err := collectDirectCodingBrowserEventBindings(function, source)
	if err != nil {
		return directCodingBrowserOutputDataflow{}, err
	}
	flow.eventBindings = eventBindings
	flow.collectInteractionSetters(render)
	flow.resolveInteractionSetters()
	return flow, nil
}

func directCodingBrowserNearestContainingFunction(node *treesitter.Node) *treesitter.Node {
	for current := node; current != nil; current = current.Parent() {
		if javaScriptFunctionScopeKind(current.Kind()) {
			return current
		}
	}
	return nil
}

func (flow *directCodingBrowserOutputDataflow) add(
	node *treesitter.Node,
	kind directCodingBrowserOutputDefinitionKind,
	value *treesitter.Node,
	scope *treesitter.Node,
	available uint,
) *directCodingBrowserOutputDefinition {
	if node == nil || scope == nil {
		return nil
	}
	name := directCodingBrowserRuntimeNodeText(flow.source, node)
	if name == "" {
		return nil
	}
	definition := &directCodingBrowserOutputDefinition{
		id: node.Id(), nodeID: node.Id(), kind: kind, value: value,
		available: available, scopeStart: scope.StartByte(), scopeEnd: scope.EndByte(),
	}
	flow.definitions[name] = append(flow.definitions[name], definition)
	return definition
}

func (flow *directCodingBrowserOutputDataflow) collectFunction(
	function *treesitter.Node,
	root bool,
) {
	if function == nil {
		return
	}
	flow.collectParameterPattern(function.ChildByFieldName("parameters"), function, root)
	flow.collectDeclarations(function.ChildByFieldName("body"))
}
