package assemblyline

import (
	"fmt"
	"strings"
)

func renderImmutableUserRequestModelProjection(userRequest string) string {
	return "IMMUTABLE USER REQUEST:\n" + userRequest
}

// renderApplicationContextModelProjection exposes only the semantic facts a
// station needs. ApplicationContext retains provenance and identity metadata
// for code-owned validation, but that metadata is not model context.
func renderApplicationContextModelProjection(
	userRequest string,
	context ApplicationContext,
) string {
	var projection strings.Builder
	fmt.Fprintf(&projection, "%s\n", renderImmutableUserRequestModelProjection(userRequest))
	for _, fact := range context.Facts {
		fmt.Fprintf(
			&projection,
			"FACT KIND:\n%s\nFACT VALUE:\n%s\n",
			fact.Kind,
			fact.Value,
		)
	}
	return strings.TrimSuffix(projection.String(), "\n")
}
