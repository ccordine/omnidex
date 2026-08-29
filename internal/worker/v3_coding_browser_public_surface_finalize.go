package worker

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (extractor *directCodingBrowserPublicSurfaceExtractor) finish() (
	directCodingBrowserPublicInteractionSurface,
	error,
) {
	byID := make(map[string][]int)
	for index, control := range extractor.controls {
		if control.id != "" {
			byID[control.id] = append(byID[control.id], index)
		}
	}
	for labelIndex, label := range extractor.labels {
		if len(label.controls) > 1 {
			return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf(
				"browser public surface rejects a label containing multiple controls",
			)
		}
		if label.forID == "" {
			continue
		}
		matches := byID[label.forID]
		if len(matches) != 1 {
			return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf(
				"browser public surface requires one exact label target",
			)
		}
		control := &extractor.controls[matches[0]]
		if control.label >= 0 && control.label != labelIndex {
			return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf(
				"browser public surface rejects ambiguous control labels",
			)
		}
		control.label = labelIndex
	}
	return extractor.finalizeControls()
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) finalizeControls() (
	directCodingBrowserPublicInteractionSurface,
	error,
) {
	roleCounts := make(map[string]int)
	for _, control := range extractor.controls {
		roleCounts[control.Role]++
	}
	roleOrdinals := make(map[string]int)
	controls := make([]directCodingBrowserPublicControl, len(extractor.controls))
	for index, pending := range extractor.controls {
		control := pending.directCodingBrowserPublicControl
		if control.AccessibleName == "" && pending.label >= 0 {
			control.AccessibleName = extractor.labels[pending.label].literal
		}
		if control.AccessibleName == "" {
			control.AccessibleName = pending.buttonText
		}
		for _, literal := range []string{control.AccessibleName, control.PlaceholderHint} {
			if err := validateDirectCodingBrowserPublicLiteral(literal); err != nil {
				return directCodingBrowserPublicInteractionSurface{}, err
			}
		}
		roleOrdinals[control.Role]++
		control.RoleOrdinal = roleOrdinals[control.Role]
		control.RoleCount = roleCounts[control.Role]
		controls[index] = control
	}
	return directCodingBrowserPublicInteractionSurface{
		Controls: controls, Outputs: extractor.outputs,
		ElementIDs: append([]string(nil), extractor.ids...),
	}, nil
}

func validateDirectCodingBrowserPublicLiteral(value string) error {
	if len(value) > directCodingBrowserPublicSurfaceMaxLiteralBytes {
		return fmt.Errorf(
			"browser public surface literal exceeds %d bytes",
			directCodingBrowserPublicSurfaceMaxLiteralBytes,
		)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("browser public surface literal is not valid UTF-8")
	}
	return nil
}

func normalizeDirectCodingBrowserPublicLiteral(value string) string {
	return strings.Join(strings.Fields(html.UnescapeString(value)), " ")
}

func treeSitterNodeContainsKind(node *treesitter.Node, kinds ...string) bool {
	if node == nil {
		return false
	}
	for _, kind := range kinds {
		if node.Kind() == kind {
			return true
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if treeSitterNodeContainsKind(node.NamedChild(index), kinds...) {
			return true
		}
	}
	return false
}
