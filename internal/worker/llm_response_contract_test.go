package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestLLMResponseContractIsSelectedByInternalJobType(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		format     string
		protocol   llm.ExactPreparedProtocol
		maxTokens  int
		outputMode llm.ExactPreparedOutputLimitMode
		promptHint string
		stop       string
	}{
		{
			name:       "portable semantic station",
			scope:      "portable_semantic_worker",
			format:     llm.ResponseFormatJSON,
			protocol:   llm.ExactPreparedProtocolStructuredV1,
			maxTokens:  1024,
			outputMode: llm.ExactPreparedOutputLimitExplicit,
			promptHint: llm.MinimalGeneratePrompt,
			stop:       "",
		},
		{
			name:       "portable fragment station",
			scope:      "portable_fragment_worker",
			format:     "",
			protocol:   llm.ExactPreparedProtocolRawTextV1,
			maxTokens:  4096,
			outputMode: llm.ExactPreparedOutputLimitNatural,
			promptHint: llm.MinimalGeneratePrompt,
			stop:       llm.ExactPreparedCodeStopV1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract, err := llmResponseContractForScope(test.scope)
			if err != nil {
				t.Fatal(err)
			}
			if contract.Protocol != test.protocol || contract.Format != test.format || contract.MaxTokens != test.maxTokens || contract.OutputLimitMode != test.outputMode || contract.PromptHint != test.promptHint || contract.RawTextStopSequence != test.stop {
				t.Fatalf("llmResponseContractForScope(%q)=%#v", test.scope, contract)
			}
		})
	}
}

func TestLLMResponseContractRejectsUnregisteredScope(t *testing.T) {
	if _, err := llmResponseContractForScope("legacy_guessing"); err == nil {
		t.Fatal("unregistered LLM scope was accepted")
	}
}

func TestEveryRawFragmentCallUsesNaturalCompletionWithoutRegionalSchema(t *testing.T) {
	generation, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Signature: "function apply(): void",
		Behavior: "Apply the one accepted behavior.",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function apply(): void",
		CurrentDeclaration: "function apply(): void { broken(); }",
		RequiredChange:     "Fix the one syntax error.", Diagnostic: "syntax error",
	})
	if err != nil {
		t.Fatal(err)
	}
	goCorrection, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "go", Signature: "func apply()",
		CurrentDeclaration: "func apply() { broken() }",
		RequiredChange:     "Fix the one syntax error.", Diagnostic: "syntax error",
	})
	if err != nil {
		t.Fatal(err)
	}
	regionCorrection, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function apply(): void",
		RepairRegion: &assemblyline.TypeScriptFragmentRepairRegion{
			StartLine: 2, EndLine: 2, Source: "  broken();",
		},
		RequiredChange: "Fix the one syntax error.", Diagnostic: "syntax error at line 2",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		job  assemblyline.PortableJob
		want int
	}{
		{name: "initial", job: generation, want: 4096},
		{name: "correction", job: correction, want: 4096},
		{name: "localized correction", job: regionCorrection, want: 4096},
		{name: "go correction", job: goCorrection, want: 4096},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, schema, renderErr := assemblyline.RenderPortableJob(test.job)
			if renderErr != nil {
				t.Fatal(renderErr)
			}
			contract, contractErr := llmResponseContractForPortableJob(test.job, schema)
			if contractErr != nil {
				t.Fatal(contractErr)
			}
			if contract.MaxTokens != test.want {
				t.Fatalf("max output tokens=%d want %d", contract.MaxTokens, test.want)
			}
			if schema != nil || contract.Protocol != llm.ExactPreparedProtocolRawTextV1 ||
				contract.Format != "" || contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
				t.Fatalf("raw fragment contract=%#v schema=%#v", contract, schema)
			}
		})
	}
}
