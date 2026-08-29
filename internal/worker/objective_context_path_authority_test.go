package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
)

func TestObjectiveContextCompileProjectsBoundModelInstructionAndArtifactPaths(t *testing.T) {
	t.Parallel()
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "The prior exchange mentions secret_owner.go.",
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := scriptedConversationCandidateProvider{
		contextSet: contextcompiler.CandidateSet{
			Optional: []assemblyline.ContextCandidateAuthority{candidate},
		},
	}
	station := &scriptedConversationContextStation{relevantIDs: []string{"CTX_1"}}
	authority := turnAuthority{
		JobID: 91, Pipeline: model.PipelineChat,
		Instruction:        "Inspect internal/private/secret_owner.go again.",
		ModelInstruction:   "Inspect ARTIFACT_1 again.",
		ModelArtifactPaths: []string{"internal/private/secret_owner.go"},
		ChannelMode:        model.ChannelModeAssistant,
	}
	compiled, calls, err := compileObjectiveTurnContext(
		t.Context(),
		model.Job{ID: authority.JobID, Pipeline: authority.Pipeline, Instruction: authority.Instruction},
		authority, provider, station, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(station.relevanceInputs) != 1 {
		t.Fatalf("context relevance calls=%d inputs=%d", calls, len(station.relevanceInputs))
	}
	input := station.relevanceInputs[0]
	if input.ExactInstruction != authority.ModelInstruction ||
		len(input.KnownArtifactPaths) != 1 ||
		input.KnownArtifactPaths[0] != authority.ModelArtifactPaths[0] {
		t.Fatalf("context relevance input=%#v", input)
	}
	if compiled.Instruction != authority.Instruction || compiled.ModelInstruction != authority.ModelInstruction {
		t.Fatalf("context compilation changed authority: %#v", compiled)
	}
}

func TestObjectiveContextCompileSourceDoesNotProjectRawInstructionAsModelText(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("objective_context_compilation.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"ExactInstruction:   contextInstruction",
		"ModelInstruction:   authority.ModelInstruction",
		"KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("objective context compile omitted authority binding %q", required)
		}
	}
	if strings.Contains(text, "ModelInstruction:   contextInstruction") {
		t.Fatal("objective context compile projects raw retrieval authority as model text")
	}
}
