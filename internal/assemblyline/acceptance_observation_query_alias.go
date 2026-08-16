package assemblyline

import (
	"fmt"
	"sort"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

// AcceptanceObservationQueryAliasRepair is the exact syntax fact needed to
// eliminate one local alias around an otherwise valid public screen query.
type AcceptanceObservationQueryAliasRepair struct {
	DeclarationLine int    `json:"declaration_line"`
	Identifier      string `json:"identifier"`
	QueryOperation  string `json:"query_operation"`
	ReferenceLines  []int  `json:"reference_lines"`
}

// ResolveTypeScriptAcceptanceObservationQueryAliasRepair recognizes one
// mechanically repairable acceptance shape. It does not infer product
// behavior: the rejected statement must be a single lexical declaration whose
// initializer is already a valid direct screen query.
func ResolveTypeScriptAcceptanceObservationQueryAliasRepair(
	source string,
	tsx bool,
	siteID string,
) (AcceptanceObservationQueryAliasRepair, bool, error) {
	var zero AcceptanceObservationQueryAliasRepair
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, tsx)
	if err != nil {
		return zero, false, err
	}
	var site AcceptanceObservationSite
	var locator AcceptanceObservationLocator
	found := false
	for index, candidate := range inventory.Sites {
		if candidate.ID == siteID {
			site = candidate
			locator = inventory.Locators[index]
			found = true
			break
		}
	}
	if !found || site.StatementKind != "lexical_declaration" ||
		!stringInSet("untrusted_call", site.Operations) {
		return zero, false, nil
	}

	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return zero, false, err
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return zero, false, fmt.Errorf("acceptance query-alias source contains invalid TypeScript syntax")
	}
	function, err := soleAcceptanceFunction(root)
	if err != nil {
		return zero, false, err
	}
	body := function.ChildByFieldName("body")
	node := smallestAcceptanceNodeContaining(root, locator.StartByte, locator.EndByte)
	for node != nil && node.Kind() != "lexical_declaration" {
		node = node.Parent()
	}
	if node == nil || body == nil || node.Parent() == nil || node.Parent().Id() != body.Id() ||
		node.NamedChildCount() != 1 {
		return zero, false, nil
	}
	declarator := node.NamedChild(0)
	if declarator == nil || declarator.Kind() != "variable_declarator" {
		return zero, false, nil
	}
	name := declarator.ChildByFieldName("name")
	value := declarator.ChildByFieldName("value")
	if name == nil || name.Kind() != "identifier" || value == nil {
		return zero, false, nil
	}
	call := value
	if value.Kind() == "await_expression" {
		if value.NamedChildCount() != 1 {
			return zero, false, nil
		}
		call = value.NamedChild(0)
	}
	if call == nil || call.Kind() != "call_expression" {
		return zero, false, nil
	}
	bytes := []byte(source)
	operation := acceptanceCallOperation(call, bytes)
	grammar := acceptanceObserverGrammar{
		trustedCalls: make(map[uintptr]struct{}), allowedStatements: make(map[uintptr]struct{}),
		source: bytes,
	}
	if !strings.HasPrefix(operation, "testing_library_query:") ||
		!throwingAcceptanceQuery(operation) || !grammar.allowRootedObservation(value, false) {
		return zero, false, nil
	}
	identifier := name.Utf8Text(bytes)
	referenceLines, safe := acceptanceQueryAliasReferenceLines(body, declarator, identifier, bytes)
	if !safe {
		return zero, false, nil
	}
	return AcceptanceObservationQueryAliasRepair{
		DeclarationLine: int(node.StartPosition().Row) + 1,
		Identifier:      identifier, QueryOperation: operation,
		ReferenceLines: referenceLines,
	}, true, nil
}

// RewriteTypeScriptAcceptanceObservationQueryAlias performs the syntax-only
// rewrite already proven by ResolveTypeScriptAcceptanceObservationQueryAliasRepair.
// Product meaning remains owned by the subsequent grounding review.
func RewriteTypeScriptAcceptanceObservationQueryAlias(
	source string,
	tsx bool,
	siteID string,
) (string, bool, error) {
	repair, mapped, err := ResolveTypeScriptAcceptanceObservationQueryAliasRepair(source, tsx, siteID)
	if err != nil || !mapped {
		return source, false, err
	}
	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return "", false, err
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	function, err := soleAcceptanceFunction(root)
	if err != nil {
		return "", false, err
	}
	body := function.ChildByFieldName("body")
	declaration, declarator, value, err := acceptanceQueryAliasDeclaration(
		body, repair.DeclarationLine, repair.Identifier, []byte(source),
	)
	if err != nil {
		return "", false, err
	}
	references, safe := acceptanceQueryAliasReferences(body, declarator, repair.Identifier, []byte(source))
	if !safe {
		return "", false, fmt.Errorf("acceptance query alias %q no longer has safely replaceable references", repair.Identifier)
	}
	edits := make([]acceptanceQueryAliasEdit, 0, len(references)+1)
	replacement := "(" + strings.TrimSpace(value.Utf8Text([]byte(source))) + ")"
	for _, reference := range references {
		edits = append(edits, acceptanceQueryAliasEdit{
			start: int(reference.StartByte()), end: int(reference.EndByte()), replacement: replacement,
		})
	}
	edits = append(edits, acceptanceQueryAliasEdit{
		start: int(declaration.StartByte()), end: int(declaration.EndByte()),
	})
	sort.Slice(edits, func(left, right int) bool { return edits[left].start > edits[right].start })
	rewritten := source
	lastStart := len(source)
	for _, edit := range edits {
		if edit.start < 0 || edit.end <= edit.start || edit.end > lastStart {
			return "", false, fmt.Errorf("acceptance query alias rewrite produced overlapping source edits")
		}
		rewritten = rewritten[:edit.start] + edit.replacement + rewritten[edit.end:]
		lastStart = edit.start
	}
	rewritten = strings.TrimSpace(rewritten)
	if rewritten == strings.TrimSpace(source) {
		return "", false, fmt.Errorf("acceptance query alias rewrite made no source change")
	}
	if _, err := InventoryTypeScriptAcceptanceObservations(rewritten, tsx); err != nil {
		return "", false, fmt.Errorf("validate rewritten acceptance query alias: %w", err)
	}
	return rewritten, true, nil
}

type acceptanceQueryAliasEdit struct {
	start       int
	end         int
	replacement string
}

func acceptanceQueryAliasDeclaration(
	body *treesitter.Node,
	line int,
	identifier string,
	source []byte,
) (*treesitter.Node, *treesitter.Node, *treesitter.Node, error) {
	if body == nil {
		return nil, nil, nil, fmt.Errorf("acceptance query alias function has no body")
	}
	for index := uint(0); index < body.NamedChildCount(); index++ {
		declaration := body.NamedChild(index)
		if declaration == nil || declaration.Kind() != "lexical_declaration" ||
			int(declaration.StartPosition().Row)+1 != line || declaration.NamedChildCount() != 1 {
			continue
		}
		declarator := declaration.NamedChild(0)
		if declarator == nil || declarator.Kind() != "variable_declarator" {
			continue
		}
		name := declarator.ChildByFieldName("name")
		value := declarator.ChildByFieldName("value")
		if name != nil && name.Kind() == "identifier" && name.Utf8Text(source) == identifier && value != nil {
			return declaration, declarator, value, nil
		}
	}
	return nil, nil, nil, fmt.Errorf(
		"acceptance query alias declaration for %q on line %d is no longer current", identifier, line,
	)
}

func smallestAcceptanceNodeContaining(
	node *treesitter.Node,
	start int,
	end int,
) *treesitter.Node {
	if node == nil || start < int(node.StartByte()) || end > int(node.EndByte()) {
		return nil
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if child := smallestAcceptanceNodeContaining(node.NamedChild(index), start, end); child != nil {
			return child
		}
	}
	return node
}

func acceptanceQueryAliasReferenceLines(
	body *treesitter.Node,
	declarator *treesitter.Node,
	identifier string,
	source []byte,
) ([]int, bool) {
	references, safe := acceptanceQueryAliasReferences(body, declarator, identifier, source)
	if !safe {
		return nil, false
	}
	lines := make([]int, 0, len(references))
	seenLines := make(map[int]struct{})
	for _, reference := range references {
		line := int(reference.StartPosition().Row) + 1
		if _, exists := seenLines[line]; !exists {
			seenLines[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	return lines, true
}

func acceptanceQueryAliasReferences(
	body *treesitter.Node,
	declarator *treesitter.Node,
	identifier string,
	source []byte,
) ([]*treesitter.Node, bool) {
	references := []*treesitter.Node{}
	safe := true
	var visit func(*treesitter.Node)
	visit = func(node *treesitter.Node) {
		if node == nil || !safe {
			return
		}
		if node.Kind() == "variable_declarator" && node.Id() != declarator.Id() {
			name := node.ChildByFieldName("name")
			if name != nil && name.Kind() == "identifier" && name.Utf8Text(source) == identifier {
				safe = false
				return
			}
		}
		if node.Kind() == "identifier" && node.Utf8Text(source) == identifier {
			name := declarator.ChildByFieldName("name")
			if name != nil && node.Id() != name.Id() {
				parent := node.Parent()
				if node.StartByte() <= declarator.EndByte() || parent == nil || parent.Kind() != "arguments" {
					safe = false
					return
				}
				references = append(references, node)
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			visit(node.NamedChild(index))
		}
	}
	visit(body)
	return references, safe
}
