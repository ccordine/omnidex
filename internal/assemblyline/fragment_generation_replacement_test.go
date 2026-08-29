package assemblyline

import (
	"strings"
	"testing"
)

func TestFragmentGenerationReplacementPreservesUnrelatedSourceResponsibilities(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name  string
		input FragmentGenerationInput
		base  func(FragmentGenerationInput) (string, error)
	}{
		{
			name: "typescript guest label",
			input: FragmentGenerationInput{
				Language: "typescript", Dialect: "TypeScript 5.9.3",
				Signature: "function formatGuestLabel(name: string, checkedIn: boolean): string",
				Behavior:  "Return the guest name with a checked-in suffix only when checked in.",
			},
			base: func(input FragmentGenerationInput) (string, error) {
				return BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
					Dialect: input.Dialect, Signature: input.Signature,
					Contract: input.Behavior,
				})
			},
		},
		{
			name: "go retry delay",
			input: FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24",
				Signature: "func RetryDelay(attempt int) int",
				Behavior:  "Return zero before the first attempt and twice the attempt otherwise.",
			},
			base: BuildGoFragmentGenerationPrompt,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			origin, err := NewFragmentGenerationJob(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			originPrompt, err := RenderPortableJob(origin)
			if err != nil {
				t.Fatal(err)
			}
			priorPrompt, err := fixture.base(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			if originPrompt != priorPrompt {
				t.Fatalf("existing fragment envelope changed:\nGOT:\n%s\nWANT:\n%s", originPrompt, priorPrompt)
			}

			replacement, err := NewFragmentGenerationReplacementJob(
				FragmentGenerationReplacementInput{
					Original: fixture.input,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(replacement)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(prompt, originPrompt+"\n\nEXACT_OUTPUT_LIMIT_EVIDENCE:\n") ||
				!strings.Contains(prompt, "reached the provider output boundary") ||
				strings.Contains(prompt, "partial-provider-source") ||
				strings.Contains(string(replacement.Payload), "partial-provider-source") ||
				strings.Contains(string(replacement.Payload), "origin_work_id") ||
				strings.Contains(string(replacement.Payload), "output_tokens") ||
				strings.Contains(string(replacement.Payload), "content_bytes") {
				t.Fatalf("replacement envelope=%q payload=%s", prompt, replacement.Payload)
			}
			framing, err := PortableResponseFramingForJob(replacement)
			if err != nil || framing != PortableResponseFramingNaturalMultiline {
				t.Fatalf("replacement framing=%q error=%v", framing, err)
			}
			transport, err := PortableResponseTransportForWorkKind(replacement.Kind)
			if err != nil || transport != PortableResponseTransportFragmentRaw {
				t.Fatalf("replacement transport=%q error=%v", transport, err)
			}
			originMaximum, err := PortableResponseMaximumBytesForJob(origin)
			if err != nil {
				t.Fatal(err)
			}
			replacementMaximum, err := PortableResponseMaximumBytesForJob(replacement)
			if err != nil || replacementMaximum != originMaximum {
				t.Fatalf(
					"replacement maximum=%d origin=%d error=%v",
					replacementMaximum, originMaximum, err,
				)
			}
		})
	}
}

func TestFragmentGenerationReplacementIdentityContainsOnlyUnresolvedSource(t *testing.T) {
	t.Parallel()
	input := FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func RetryDelay(attempt int) int",
		Behavior: "Return one retry delay.",
	}
	first, err := NewFragmentGenerationReplacementJob(
		FragmentGenerationReplacementInput{Original: input},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFragmentGenerationReplacementJob(
		FragmentGenerationReplacementInput{Original: input},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || strings.Contains(string(first.Payload), "origin_") ||
		strings.Contains(string(first.Payload), "output_limit") {
		t.Fatalf("replacement identity leaked operational provenance: %+v", first)
	}
}
