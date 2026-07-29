package worker

import (
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func orderGroundedQuotes(authority string, quotes []string) []string {
	ordered := append([]string(nil), quotes...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return strings.Index(authority, ordered[left]) < strings.Index(authority, ordered[right])
	})
	return ordered
}

func requirementSourceQuotes(requirements []assemblyline.Requirement) []string {
	quotes := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		quotes = append(quotes, requirement.SourceQuote)
	}
	return quotes
}
