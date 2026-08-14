package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func TestRawCodingPromptsAdmitExactDirectCapabilitiesBeyondLegacyTwoKiB(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name       string
		capability string
		job        func(string) (PortableJob, error)
	}{
		{
			name:       "calendar generation",
			capability: largeTypeScriptCapability("CalendarSnapshot", "entry"),
			job: func(capability string) (PortableJob, error) {
				return NewFragmentGenerationJob(FragmentGenerationInput{
					Language:         "typescript",
					Signature:        "function firstCalendarEntry(value: CalendarSnapshot): string",
					Behavior:         "Return the first available calendar entry.",
					Capabilities:     []string{capability},
					PermittedSymbols: []string{"CalendarSnapshot"},
				})
			},
		},
		{
			name:       "geometry correction",
			capability: largeTypeScriptCapability("MeshVertexCatalog", "vertex"),
			job: func(capability string) (PortableJob, error) {
				return NewFragmentCorrectionJob(FragmentCorrectionInput{
					Language:           "typescript",
					Signature:          "function vertexCount(value: MeshVertexCatalog): number",
					Capabilities:       []string{capability},
					PermittedSymbols:   []string{"MeshVertexCatalog"},
					CurrentDeclaration: "function vertexCount(value: MeshVertexCatalog): number { return 0; }",
					RequiredChange:     "Return the catalog vertex count.",
					Diagnostic:         "expected the catalog count, received zero",
				})
			},
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			if len(fixture.capability) <= 2*1024 {
				t.Fatalf("capability fixture=%dB; expected more than 2 KiB", len(fixture.capability))
			}
			job, err := fixture.job(fixture.capability)
			if err != nil {
				t.Fatalf("exact direct capability was rejected: %v", err)
			}
			prompt, schema, err := RenderPortableJob(job)
			if err != nil {
				t.Fatalf("render raw coding prompt: %v", err)
			}
			if schema != nil {
				t.Fatal("raw coding prompt unexpectedly requested structured output")
			}
			if strings.Count(prompt, fixture.capability) != 1 {
				t.Fatal("prompt did not preserve the exact direct capability once")
			}
			if len(prompt) >= maxPortableResourceBytes {
				t.Fatalf("prompt=%dB crossed the coarse portable resource ceiling", len(prompt))
			}
		})
	}
}

func TestExpandedRawCodingCapabilitySizeRetainsExactSetAndSymbolAuthority(t *testing.T) {
	t.Parallel()

	base := FragmentGenerationInput{
		Language: "typescript", Signature: "function readValue(): number",
		Behavior:         "Return the permitted value.",
		Capabilities:     []string{"declare const PermittedValue: number;"},
		PermittedSymbols: []string{"PermittedValue"},
	}
	for name, mutate := range map[string]func(*FragmentGenerationInput){
		"empty capability": func(input *FragmentGenerationInput) {
			input.Capabilities = []string{""}
		},
		"untrimmed capability": func(input *FragmentGenerationInput) {
			input.Capabilities = []string{" declare const PermittedValue: number;"}
		},
		"duplicate capability": func(input *FragmentGenerationInput) {
			input.Capabilities = append(input.Capabilities, input.Capabilities[0])
		},
		"oversized symbol authority": func(input *FragmentGenerationInput) {
			input.PermittedSymbols = []string{strings.Repeat("s", 1025)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := NewFragmentGenerationJob(input); err == nil {
				t.Fatal("invalid direct capability authority was accepted")
			}
		})
	}
}

func largeTypeScriptCapability(typeName, fieldPrefix string) string {
	var capability strings.Builder
	capability.WriteString("interface ")
	capability.WriteString(typeName)
	capability.WriteString(" {\n")
	for index := 0; capability.Len() <= 3*1024; index++ {
		fmt.Fprintf(&capability, "  readonly %s%04d: string;\n", fieldPrefix, index)
	}
	capability.WriteString("}")
	return capability.String()
}
