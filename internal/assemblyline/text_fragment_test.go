package assemblyline

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextFragmentJobRendersOneBoundedRawTextNode(t *testing.T) {
	t.Parallel()

	job, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language:  TextFragmentLanguage,
		Dialect:   TextFragmentDialect,
		Signature: TextFragmentSignature,
		Behavior:  "State that the deterministic proof completed successfully.",
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"exactly one raw UTF-8 text node",
		"End the node with exactly one LF byte",
		"EXACT_LOCAL_BEHAVIOR:",
		"State that the deterministic proof completed successfully.",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("text fragment prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"file path", "workspace", "create_file", "write_file", "completion status",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("text fragment prompt exposed %q:\n%s", forbidden, prompt)
		}
	}
	if schema != nil {
		t.Fatalf("raw text fragment unexpectedly has a response schema: %#v", schema)
	}
}

func TestTextFragmentInputRejectsNonTextAuthority(t *testing.T) {
	t.Parallel()

	valid := FragmentGenerationInput{
		Language:  TextFragmentLanguage,
		Dialect:   TextFragmentDialect,
		Signature: TextFragmentSignature,
		Behavior:  "Return one short proof statement.",
	}
	for name, mutate := range map[string]func(*FragmentGenerationInput){
		"dialect": func(input *FragmentGenerationInput) {
			input.Dialect = "arbitrary prose"
		},
		"signature": func(input *FragmentGenerationInput) {
			input.Signature = "whole_document"
		},
		"capability": func(input *FragmentGenerationInput) {
			input.Capabilities = []string{"runtime.api"}
		},
		"symbol": func(input *FragmentGenerationInput) {
			input.PermittedSymbols = []string{"value"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := BuildTextFragmentGenerationPrompt(input); err == nil {
				t.Fatal("invalid text fragment authority was accepted")
			}
		})
	}
}

func TestProjectTextFragmentPreservesTheExactResponse(t *testing.T) {
	t.Parallel()

	raw := "Deterministic proof: café ✓  \n"
	projection, err := ProjectTextFragment(raw)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Kind != PortableResultProjectionExactResponse ||
		projection.Source != raw || projection.StartByte != 0 ||
		projection.EndByte != len(raw) || projection.RawBytes != len(raw) ||
		projection.DiscardedBytes != 0 {
		t.Fatalf("projection=%+v", projection)
	}
	if err := projection.ValidateFor(raw); err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateFor(raw + "changed\n"); err == nil {
		t.Fatal("exact text projection accepted a different raw response")
	}
}

func TestValidateTextFragmentEnforcesUTF8NULAndNewlinePolicy(t *testing.T) {
	t.Parallel()

	invalidUTF8 := string([]byte{'o', 'k', 0xff, '\n'})
	if utf8.ValidString(invalidUTF8) {
		t.Fatal("invalid UTF-8 fixture is valid")
	}
	for name, candidate := range map[string]string{
		"empty":                 "",
		"invalid UTF-8":         invalidUTF8,
		"NUL":                   "proof\x00\n",
		"carriage return":       "proof\r\n",
		"missing terminal LF":   "proof",
		"multiple terminal LFs": "proof\n\n",
		"empty body":            "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateTextFragment(candidate); err == nil {
				t.Fatalf("invalid text fragment %q was accepted", candidate)
			}
			if _, err := ProjectTextFragment(candidate); err == nil {
				t.Fatalf("invalid text fragment %q was projected", candidate)
			}
		})
	}

	valid := "first line\n\nsecond line with trailing spaces  \n"
	if err := ValidateTextFragment(valid); err != nil {
		t.Fatalf("valid text fragment rejected: %v", err)
	}
}
