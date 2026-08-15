package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func exactTypeScriptReplayHeader(input assemblyline.FragmentCorrectionInput) (string, error) {
	reactValues := make([]string, 0)
	reactTypes := make([]string, 0)
	testingValues := make([]string, 0)
	for _, symbol := range input.PermittedSymbols {
		switch symbol {
		case "ReactElement", "ReactNode", "CSSProperties":
			reactTypes = append(reactTypes, symbol)
		case "useCallback", "useEffect", "useMemo", "useRef", "useState":
			reactValues = append(reactValues, symbol)
		case "fireEvent", "screen", "waitFor":
			testingValues = append(testingValues, symbol)
		case "expect":
			// Vitest exposes expect through the pinned tsconfig type authority.
		default:
			return "", fmt.Errorf("TypeScript replay cannot reconstruct permitted symbol %q", symbol)
		}
	}
	parts := make([]string, 0, len(input.Capabilities)+3)
	if len(reactValues) > 0 {
		parts = append(parts, "import { "+strings.Join(reactValues, ", ")+" } from 'react';")
	}
	if len(reactTypes) > 0 {
		parts = append(parts, "import type { "+strings.Join(reactTypes, ", ")+" } from 'react';")
	}
	if len(testingValues) > 0 {
		parts = append(parts, "import { "+strings.Join(testingValues, ", ")+" } from '@testing-library/react';")
	}
	for _, capability := range input.Capabilities {
		if err := assemblyline.ValidateTypeScriptSource(capability, true); err != nil {
			return "", fmt.Errorf("validate TypeScript replay capability: %w", err)
		}
		parts = append(parts, capability)
	}
	return strings.Join(parts, "\n"), nil
}
