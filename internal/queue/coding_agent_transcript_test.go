package queue

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

func TestLongCodingResponseProjectsWithoutTruncationOrInvalidUTF8(t *testing.T) {
	for _, text := range []string{"ordinary response", strings.Repeat("界", 30000), "start" + strings.Repeat(" ", 100000) + "end"} {
		parts, err := codingAssistantMessageParts(text)
		if err != nil {
			t.Fatal(err)
		}
		var restored strings.Builder
		for index, part := range parts {
			if len(part) > model.MaxChannelContentBytes || !utf8.ValidString(part) || strings.TrimSpace(part) == "" {
				t.Fatalf("invalid transcript part %d: bytes=%d", index, len(part))
			}
			if len(parts) > 1 {
				part = strings.TrimPrefix(part, fmt.Sprintf("Response (part %d of %d)\n", index+1, len(parts)))
			}
			restored.WriteString(part)
		}
		if restored.String() != text {
			t.Fatal("response bytes were dropped or duplicated")
		}
	}
}
