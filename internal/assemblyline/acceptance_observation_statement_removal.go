package assemblyline

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

// RemoveTypeScriptAcceptanceObservationStatement removes the smallest complete
// statement containing one code-identified acceptance observation. The caller
// supplies the immutable inventory identity; this function resolves its exact
// current AST span and never asks a model to reproduce a known deletion.
func RemoveTypeScriptAcceptanceObservationStatement(
	source string,
	tsx bool,
	siteID string,
) (string, bool, error) {
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, tsx)
	if err != nil {
		return "", false, err
	}
	locator, found := acceptanceObservationLocatorForSite(inventory, siteID)
	if !found {
		return "", false, fmt.Errorf("acceptance observation site %s is not present in the current declaration", siteID)
	}
	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return "", false, err
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return "", false, fmt.Errorf("acceptance observation source contains invalid TypeScript syntax")
	}
	if _, err := soleAcceptanceFunction(root); err != nil {
		return "", false, err
	}
	statement := acceptanceObservationContainingStatement(root, locator)
	if statement == nil {
		return "", false, fmt.Errorf("acceptance observation site %s has no removable statement", siteID)
	}
	start, end := int(statement.StartByte()), int(statement.EndByte())
	if start < 0 || end <= start || end > len(source) ||
		start > locator.StartByte || end < locator.EndByte {
		return "", false, fmt.Errorf("acceptance observation site %s has an invalid statement span", siteID)
	}
	corrected := source[:start] + source[end:]
	if corrected == source {
		return "", false, fmt.Errorf("acceptance observation statement removal made no source change")
	}
	validationParser, validationTree, err := parseTypeScriptResponseTree(corrected, tsx)
	if err != nil {
		return "", false, fmt.Errorf("parse statement removal result: %w", err)
	}
	defer validationParser.Close()
	defer validationTree.Close()
	if validationTree.RootNode().HasError() {
		return "", false, fmt.Errorf("acceptance observation statement removal produced invalid TypeScript syntax")
	}
	if _, err := soleAcceptanceFunction(validationTree.RootNode()); err != nil {
		return "", false, fmt.Errorf("validate statement removal declaration: %w", err)
	}
	return corrected, true, nil
}

func acceptanceObservationLocatorForSite(
	inventory AcceptanceObservationInventory,
	siteID string,
) (AcceptanceObservationLocator, bool) {
	for _, locator := range inventory.Locators {
		if locator.SiteID == siteID {
			return locator, true
		}
	}
	return AcceptanceObservationLocator{}, false
}

func acceptanceObservationContainingStatement(
	root *treesitter.Node,
	locator AcceptanceObservationLocator,
) *treesitter.Node {
	node := smallestAcceptanceNodeContaining(root, locator.StartByte, locator.EndByte)
	for node != nil {
		parent := node.Parent()
		if parent != nil && parent.Kind() == "statement_block" {
			return node
		}
		node = parent
	}
	return nil
}
