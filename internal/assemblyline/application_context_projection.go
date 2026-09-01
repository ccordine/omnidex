package assemblyline

import (
	"strings"
)

func renderImmutableUserRequestModelProjection(userRequest string) string {
	return "Software request:\n" + userRequest
}

// renderApplicationContextModelProjection exposes only the semantic facts a
// station needs. ApplicationContext retains provenance and identity metadata
// for code-owned validation, but that metadata is not model context.
func renderApplicationContextModelProjection(
	userRequest string,
	context ApplicationContext,
) string {
	projection := []string{renderImmutableUserRequestModelProjection(userRequest)}
	for _, fact := range context.Facts {
		projection = append(projection, "Established fact:\n"+fact.Value)
	}
	return strings.Join(projection, "\n\n")
}
