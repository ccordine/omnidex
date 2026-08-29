package taskstate

import (
	"strings"
	"testing"
)

func TestFeedbackEntryContentPreservesExactNonblankBytes(t *testing.T) {
	exact := "  Preserve the accepted user feedback exactly.  \n"
	if err := requireEntryContent(exact, EntryFeedback); err != nil {
		t.Fatalf("exact feedback rejected: %v", err)
	}
	if err := requireEntryContent(exact, EntryNote); err == nil {
		t.Fatal("ordinary entry accepted surrounding whitespace")
	}
	for name, value := range map[string]string{
		"empty":       "",
		"blank":       " \t\r\n",
		"nul":         "invalid\x00feedback",
		"invalidUTF8": string([]byte{0xff}),
		"oversized":   strings.Repeat("x", maxFeedbackEntryContentBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := requireEntryContent(value, EntryFeedback); err == nil {
				t.Fatal("invalid feedback entry content accepted")
			}
		})
	}
}
