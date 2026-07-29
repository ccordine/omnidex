package assemblyline

import (
	"fmt"
	"strings"
)

type textSpan struct {
	Start int
	End   int
}

func (span textSpan) Contains(other textSpan) bool {
	return span.Start <= other.Start && span.End >= other.End
}

func (span textSpan) Overlaps(other textSpan) bool {
	return span.Start < other.End && other.Start < span.End
}

func uniqueTextSpan(source, quote string) (textSpan, error) {
	start := strings.Index(source, quote)
	if start < 0 {
		return textSpan{}, fmt.Errorf("is not an exact source substring")
	}
	if strings.Contains(source[start+len(quote):], quote) {
		return textSpan{}, fmt.Errorf("occurs more than once; select a uniquely grounded longer quote")
	}
	return textSpan{Start: start, End: start + len(quote)}, nil
}
