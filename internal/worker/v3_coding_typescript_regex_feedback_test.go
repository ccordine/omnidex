package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestTypeScriptDiagnosticRegularExpressionsUseLosslessPathFreeProjection(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]struct {
		literal    string
		required   []string
		spaceCount int
	}{
		"inventory repeated whitespace": {
			literal:    `/out  stock/i`,
			required:   []string{`source text "out"`, "one space character (U+0020)", `source text "stock"`, `source flag "i": case-insensitive matching`},
			spaceCount: 2,
		},
		"schedule escaped solidus": {
			literal:  `/^monday\/tuesday$/u`,
			required: []string{"circumflex-accent character (U+005E)", `source text "monday"`, "backslash (reverse solidus) character (U+005C)", "forward-slash (solidus) character (U+002F)", `source text "tuesday"`, "dollar-sign character (U+0024)", `source flag "u": Unicode matching`},
		},
		"path shaped source bytes": {
			literal:  `/C:\\tmp\/~private/i`,
			required: []string{`source text "C"`, "source byte hexadecimal 3A", "backslash (reverse solidus) character (U+005C)", `source text "tmp"`, "forward-slash (solidus) character (U+002F)", "source byte hexadecimal 7E", `source text "private"`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := "Unable to find the matcher `" + fixture.literal + "`."
			projected, err := canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions(
				input, []string{fixture.literal},
			)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(projected, fixture.literal) || modelcontext.ContainsPathIdentity(projected) {
				t.Fatalf("projection retained path-shaped literal: %q", projected)
			}
			for _, required := range fixture.required {
				if !strings.Contains(projected, required) {
					t.Fatalf("projection omitted %q: %q", required, projected)
				}
			}
			if fixture.spaceCount > 0 && strings.Count(projected, "one space character (U+0020)") != fixture.spaceCount {
				t.Fatalf("projection changed repeated whitespace: %q", projected)
			}
			if !strings.Contains(projected, "Words and numeric labels describing a component are not pattern text") ||
				strings.Contains(projected, "TEXT_") || strings.Contains(projected, "BYTE_") {
				t.Fatalf("projection retained ambiguous component notation: %q", projected)
			}
		})
	}
}

func TestTypeScriptDiagnosticPlainRegexUsesCompactUnambiguousProjection(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]struct {
		literal string
		want    string
	}{
		"inventory action": {
			literal: `/restock/i`,
			want:    `plain text "restock" (case-insensitive)`,
		},
		"scheduling phrase": {
			literal: `/next appointment/i`,
			want:    `plain text "next appointment" (case-insensitive)`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			projected, err := canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions(
				"Unable to find name `"+fixture.literal+"`.", []string{fixture.literal},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(projected, fixture.want) ||
				strings.Contains(projected, "ordered components") ||
				strings.Contains(projected, "source text") ||
				strings.Contains(projected, "source flag") ||
				strings.Contains(projected, "U+") ||
				strings.Contains(projected, fixture.literal) ||
				modelcontext.ContainsPathIdentity(projected) {
				t.Fatalf("plain regex projection is ambiguous: %q", projected)
			}
		})
	}
}

func TestTypeScriptDiagnosticPlainRegexProjectionRejectsLossyOrSemanticSyntax(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ pattern, flags string }{
		{pattern: ""}, {pattern: " leading"}, {pattern: "trailing "},
		{pattern: "two  spaces"}, {pattern: "tab\ttext"}, {pattern: "out-of-stock"},
		{pattern: "^anchored$"}, {pattern: `digit\\d`},
		{pattern: "plain", flags: "g"}, {pattern: "plain", flags: "u"},
		{pattern: "plain", flags: "gi"},
	} {
		if directCodingRegularExpressionPlainSourceText(fixture.pattern, fixture.flags) {
			t.Fatalf("accepted non-plain or lossy pattern %q flags %q", fixture.pattern, fixture.flags)
		}
	}
	for _, fixture := range []struct{ pattern, flags string }{
		{pattern: "restock"},
		{pattern: "next appointment", flags: "i"},
		{pattern: "Version 2", flags: "i"},
	} {
		if !directCodingRegularExpressionPlainSourceText(fixture.pattern, fixture.flags) {
			t.Fatalf("rejected compact lossless plain pattern %q flags %q", fixture.pattern, fixture.flags)
		}
	}
}

func TestTypeScriptDiagnosticRegexByteDescriptionsAreExactDistinctAndPathFree(t *testing.T) {
	t.Parallel()
	seen := make(map[string]byte, 256)
	for value := 0; value <= 0xff; value++ {
		description := describeDirectCodingRegularExpressionByte(byte(value))
		hex := fmt.Sprintf("%02X", value)
		codePoint := fmt.Sprintf("U+%04X", value)
		if !strings.Contains(description, hex) && !strings.Contains(description, codePoint) {
			t.Fatalf("byte %s description is not exact: %q", hex, description)
		}
		if prior, exists := seen[description]; exists {
			t.Fatalf("bytes %02X and %02X share description %q", prior, value, description)
		}
		seen[description] = byte(value)
		if strings.ContainsAny(description, `/\\`) || modelcontext.ContainsPathIdentity(description) {
			t.Fatalf("byte %s description retained path syntax: %q", hex, description)
		}
	}
}

func TestTypeScriptDiagnosticRegexAuthorityRequiresCompleteToken(t *testing.T) {
	t.Parallel()
	projected, err := canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions(
		"Observed /tmp/private.ts and exact matcher `/tmp/`.", []string{`/tmp/`},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projected, "/tmp/private.ts") {
		t.Fatalf("regex prefix concealed a larger path: %q", projected)
	}
	if strings.Contains(projected, "matcher `/tmp/`") ||
		strings.Count(projected, `plain text "tmp" (case-sensitive)`) != 1 {
		t.Fatalf("exact matcher was not projected once: %q", projected)
	}
}

func TestTypeScriptDiagnosticRegexAuthorityMustBeOneExactParserNode(t *testing.T) {
	t.Parallel()
	for _, literal := range []string{"", "/unterminated", "/first/; /second/"} {
		if _, err := canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions(
			"matcher `"+literal+"`", []string{literal},
		); err == nil {
			t.Fatalf("accepted invalid regex authority %q", literal)
		}
	}
}

func TestTypeScriptStageFeedbackHasNoRegexPathException(t *testing.T) {
	t.Parallel()
	_, err := directCodingTypeScriptStageModelFeedback(&directCodingStageDiagnostic{
		BlockID: "inventory.render", ModelFeedback: "Unable to find `/out-of-stock/i`.",
	})
	if err == nil || !strings.Contains(err.Error(), "path identity") {
		t.Fatalf("raw regex-shaped prose bypassed strict stage feedback: %v", err)
	}
}
