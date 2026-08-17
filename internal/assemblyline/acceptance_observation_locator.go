package assemblyline

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func acceptanceObservationNodeLocator(
	siteID string,
	node *treesitter.Node,
) AcceptanceObservationLocator {
	start := node.StartPosition()
	end := node.EndPosition()
	return AcceptanceObservationLocator{
		SiteID: siteID, StartByte: int(node.StartByte()), EndByte: int(node.EndByte()),
		StartLine: int(start.Row) + 1, StartColumn: int(start.Column) + 1,
		EndLine: int(end.Row) + 1, EndColumn: int(end.Column) + 1,
	}
}

func ResolveTypeScriptAcceptanceObservationSite(
	source string,
	tsx bool,
	line int,
	column int,
) (string, bool, error) {
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, tsx)
	if err != nil {
		return "", false, err
	}
	offset, err := acceptanceSourceOffset(source, line, column)
	if err != nil {
		return "", false, err
	}
	sites := make(map[string]AcceptanceObservationSite, len(inventory.Sites))
	for _, site := range inventory.Sites {
		sites[site.ID] = site
	}
	selected := ""
	selectedWidth := 0
	for _, locator := range inventory.Locators {
		site := sites[locator.SiteID]
		if !acceptanceFailureRoutableSite(site) || offset < locator.StartByte || offset >= locator.EndByte {
			continue
		}
		width := locator.EndByte - locator.StartByte
		if selected == "" || width < selectedWidth {
			selected = locator.SiteID
			selectedWidth = width
		}
	}
	if selected != "" {
		return selected, true, nil
	}
	wrapper := AcceptanceObservationLocator{}
	wrapperWidth := 0
	for _, locator := range inventory.Locators {
		site := sites[locator.SiteID]
		if !acceptanceFailureWrapperSite(site) || offset < locator.StartByte || offset >= locator.EndByte {
			continue
		}
		width := locator.EndByte - locator.StartByte
		if wrapper.SiteID == "" || width < wrapperWidth {
			wrapper = locator
			wrapperWidth = width
		}
	}
	if wrapper.SiteID == "" {
		return "", false, nil
	}
	descendants := make([]AcceptanceObservationLocator, 0, 2)
	for _, locator := range inventory.Locators {
		if locator.StartByte < wrapper.StartByte || locator.EndByte > wrapper.EndByte ||
			locator.SiteID == wrapper.SiteID || !acceptanceFailureRoutableSite(sites[locator.SiteID]) {
			continue
		}
		descendants = append(descendants, locator)
	}
	if len(descendants) == 0 {
		return "", false, nil
	}
	root := descendants[0]
	for _, candidate := range descendants[1:] {
		if candidate.EndByte-candidate.StartByte > root.EndByte-root.StartByte {
			root = candidate
		}
	}
	for _, candidate := range descendants {
		if candidate.StartByte < root.StartByte || candidate.EndByte > root.EndByte {
			return "", false, nil
		}
	}
	return root.SiteID, true, nil
}

func acceptanceSourceOffset(source string, line int, column int) (int, error) {
	if line < 1 || column < 1 {
		return 0, fmt.Errorf("acceptance observation location must use positive 1-based coordinates")
	}
	lines := strings.SplitAfter(source, "\n")
	if line > len(lines) {
		return 0, fmt.Errorf("acceptance observation line %d is outside the declaration", line)
	}
	lineText := strings.TrimSuffix(lines[line-1], "\n")
	lineText = strings.TrimSuffix(lineText, "\r")
	if column > len(lineText)+1 {
		return 0, fmt.Errorf("acceptance observation column %d is outside line %d", column, line)
	}
	offset := 0
	for index := 0; index < line-1; index++ {
		offset += len(lines[index])
	}
	return offset + column - 1, nil
}

func acceptanceFailureRoutableSite(site AcceptanceObservationSite) bool {
	if len(site.Operations) == 0 || stringInSet("untrusted_call", site.Operations) {
		return false
	}
	operation := site.Operations[0]
	return strings.HasPrefix(operation, "testing_library_query:") ||
		strings.HasPrefix(operation, "expect_matcher:")
}

func acceptanceFailureWrapperSite(site AcceptanceObservationSite) bool {
	return len(site.Operations) == 1 && site.Operations[0] == "harness_call:waitFor"
}
