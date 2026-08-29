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
		protocol   llm.ExactPreparedProtocol
		outputMode llm.ExactPreparedOutputLimitMode
		promptHint string
	}{
		{
			name:       "portable semantic station",
			scope:      assemblyline.PortableSemanticWorkerScope,
			protocol:   llm.ExactPreparedProtocolRawTextV2,
			outputMode: llm.ExactPreparedOutputLimitNatural,
			promptHint: llm.MinimalGeneratePrompt,
		},
		{
			name:       "portable structural station",
			scope:      assemblyline.PortableStructuralWorkerScope,
			protocol:   llm.ExactPreparedProtocolRawTextV2,
			outputMode: llm.ExactPreparedOutputLimitNatural,
			promptHint: llm.MinimalGeneratePrompt,
		},
		{
			name:       "portable fragment station",
			scope:      assemblyline.PortableFragmentWorkerScope,
			protocol:   llm.ExactPreparedProtocolRawTextV2,
			outputMode: llm.ExactPreparedOutputLimitNatural,
			promptHint: llm.MinimalGeneratePrompt,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract, err := llmResponseContractForScope(test.scope)
			if err != nil {
				t.Fatal(err)
			}
			if contract.Protocol != test.protocol || contract.OutputLimitMode != test.outputMode ||
				contract.PromptHint != test.promptHint || contract.ResponseFraming != "" {
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
		Language: "typescript", Dialect: "TypeScript 5.9.3", Signature: "function apply(): void",
		Behavior: "Apply the one accepted behavior.",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function apply(): void",
		CurrentDeclaration: "function apply(): void { broken(); }",
		RepairGuidance:     "Fix the one syntax error.",
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
		RepairGuidance: "Fix the one syntax error.",
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
	} {
		t.Run(test.name, func(t *testing.T) {
			contract, contractErr := llmResponseContractForPortableJob(test.job)
			if contractErr != nil {
				t.Fatal(contractErr)
			}
			if contract.Protocol != llm.ExactPreparedProtocolRawTextV2 ||
				contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
				contract.ResponseFraming != assemblyline.PortableResponseFramingNaturalMultiline {
				t.Fatalf("raw fragment contract=%#v", contract)
			}
		})
	}
}

func TestPortableSingleLineSemanticCallUsesProviderLFFraming(t *testing.T) {
	t.Parallel()

	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Build a browser inventory."},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []assemblyline.PortableJob{classification} {
		contract, err := llmResponseContractForPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		if contract.Protocol != llm.ExactPreparedProtocolRawTextV2 ||
			contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
			contract.ResponseFraming != assemblyline.PortableResponseFramingSingleLine {
			t.Fatalf("raw semantic contract for %s=%#v", job.Kind, contract)
		}
	}
}

func TestPortableMultilineCallsPreserveNaturalProviderFraming(t *testing.T) {
	conversation, err := assemblyline.NewConversationResponseJob(
		assemblyline.ConversationResponseInput{
			Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Explain the subject.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := assemblyline.NewTargetTreeJob(assemblyline.TargetTreeInput{
		Objective: "Create one implementation artifact.", TechnicalContext: "plain text",
		Constraints:   assemblyline.TargetTreeConstraints{ExactPathCount: 1},
		ExistingPaths: []string{}, ReservedPaths: []string{}, ExistingDirs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []assemblyline.PortableJob{conversation, tree} {
		contract, err := llmResponseContractForPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		if contract.ResponseFraming != assemblyline.PortableResponseFramingNaturalMultiline {
			t.Fatalf("multiline work %q has framing %q", job.Kind, contract.ResponseFraming)
		}
	}
}
