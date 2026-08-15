package worker

import (
	"strings"
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
		outputMode llm.ExactPreparedOutputLimitMode
		promptHint string
		stop       string
	}{
		{
			name:       "portable semantic station",
			scope:      "portable_semantic_worker",
			format:     llm.ResponseFormatJSON,
			protocol:   llm.ExactPreparedProtocolStructuredV1,
			outputMode: llm.ExactPreparedOutputLimitNatural,
			promptHint: llm.MinimalGeneratePrompt,
			stop:       "",
		},
		{
			name:       "portable fragment station",
			scope:      "portable_fragment_worker",
			format:     "",
			protocol:   llm.ExactPreparedProtocolRawTextV1,
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
			if contract.Protocol != test.protocol || contract.Format != test.format || contract.OutputLimitMode != test.outputMode || contract.PromptHint != test.promptHint || contract.RawTextStopSequence != test.stop {
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
			Kind:      assemblyline.TypeScriptRepairRegionSyntaxWindow,
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
	}{
		{name: "initial", job: generation},
		{name: "correction", job: correction},
		{name: "localized correction", job: regionCorrection},
		{name: "go correction", job: goCorrection},
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
			if schema != nil || contract.Protocol != llm.ExactPreparedProtocolRawTextV1 ||
				contract.Format != "" || contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
				t.Fatalf("raw fragment contract=%#v schema=%#v", contract, schema)
			}
		})
	}
}

func TestEveryStructuredPortableSemanticCallUsesNaturalCompletion(t *testing.T) {
	t.Parallel()

	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Build a browser inventory."},
	)
	if err != nil {
		t.Fatal(err)
	}
	groundingInput, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		assemblyline.ApplicationTaskContext{
			WorkloadSHA256: strings.Repeat("a", 64),
			Task: assemblyline.ApplicationTaskContextTask{
				TaskID: "task_001", AcceptanceCriteria: []string{"The inventory is visible."},
			},
		},
		`function VerifyInventory(): void { expect(screen.getByText("Inventory")).toBeInTheDocument(); }`,
		true,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	grounding, err := assemblyline.NewApplicationAcceptanceGroundingReviewJob(groundingInput)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []assemblyline.PortableJob{classification, grounding} {
		_, schema, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		contract, err := llmResponseContractForPortableJob(job, schema)
		if err != nil {
			t.Fatal(err)
		}
		if schema == nil || contract.Protocol != llm.ExactPreparedProtocolStructuredV1 ||
			contract.Format != llm.ResponseFormatJSON ||
			contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
			t.Fatalf("structured natural contract for %s=%#v schema=%#v", job.Kind, contract, schema)
		}
	}
}
