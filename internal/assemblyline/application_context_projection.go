package assemblyline

import (
	"fmt"
	"strings"
)

// renderApplicationContextModelProjection exposes only the semantic facts a
// station needs. ApplicationContext retains provenance and identity metadata
// for code-owned validation, but that metadata is not model context.
func renderApplicationContextModelProjection(
	userRequest string,
	context ApplicationContext,
) string {
	var projection strings.Builder
	fmt.Fprintf(&projection, "IMMUTABLE USER REQUEST:\n%s\n", userRequest)
	fmt.Fprintf(&projection, "WORKSPACE STATE:\n%s\n", context.WorkspaceState)
	for _, fact := range context.Facts {
		if fact.Kind == ApplicationContextWorkspaceState {
			continue
		}
		fmt.Fprintf(
			&projection,
			"FACT KIND:\n%s\nFACT VALUE:\n%s\n",
			fact.Kind,
			fact.Value,
		)
	}
	return strings.TrimSuffix(projection.String(), "\n")
}
