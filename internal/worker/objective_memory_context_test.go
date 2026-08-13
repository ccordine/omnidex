package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestObjectiveMemoryContextZeroCandidatesMakesZeroSelectorCalls(t *testing.T) {
	selector := &scriptedConversationContextStation{}
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 901, Pipeline: model.PipelineChat, Instruction: "Answer exactly.", CurrentGeneration: 1,
	}, scriptedConversationCandidateProvider{}, selector, kind, conversation,
		&scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err != nil {
		t.Fatal(err)
	}
	if selector.memoryCalls != 0 || result.ModelCalls != 2 {
		t.Fatalf("memory selector calls=%d result=%+v", selector.memoryCalls, result)
	}
}

func TestObjectiveMemorySelectionProjectsOnlyOriginalIDCapsules(t *testing.T) {
	first := memoryContextCandidate(11, "immutable first")
	second := memoryContextCandidate(12, "immutable second")
	provider := scriptedConversationCandidateProvider{memory: []assemblyline.MemoryContextCandidate{first, second}}
	selector := &scriptedConversationContextStation{memoryDecision: assemblyline.MemoryContextSelectionDecision{
		Schema: assemblyline.MemoryContextSelectionSchemaV1, ReferencedMemoryIDs: []int64{12},
	}}
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{}
	_, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 902, Pipeline: model.PipelineAssistant, Instruction: "Use relevant continuity.", CurrentGeneration: 1,
	}, provider, selector, kind, conversation, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err != nil {
		t.Fatal(err)
	}
	for _, context := range []assemblyline.ObjectiveContext{kind.input.Context, conversation.input.Context} {
		if len(context.MemoryAuthorities) != 1 || context.MemoryAuthorities[0].MemoryID != second.MemoryID ||
			context.MemoryAuthorities[0].Content != second.Content ||
			context.MemoryAuthorities[0].ContentSHA256 != second.ContentSHA256 {
			t.Fatalf("projected memory=%+v", context.MemoryAuthorities)
		}
	}
	if selector.memoryCalls != 1 {
		t.Fatalf("memory selector calls=%d", selector.memoryCalls)
	}
}

func TestInvalidMemoryIDFailsBeforeObjectiveClassification(t *testing.T) {
	provider := scriptedConversationCandidateProvider{memory: []assemblyline.MemoryContextCandidate{
		memoryContextCandidate(21, "bounded memory"),
	}}
	selector := &scriptedConversationContextStation{memoryDecision: assemblyline.MemoryContextSelectionDecision{
		Schema: assemblyline.MemoryContextSelectionSchemaV1, ReferencedMemoryIDs: []int64{22},
	}}
	kind := answerObjectiveKindStation()
	_, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 903, Pipeline: model.PipelineChat, Instruction: "Continue.", CurrentGeneration: 1,
	}, provider, selector, kind, &scriptedObjectiveConversationStation{},
		&scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err == nil || kind.calls != 0 {
		t.Fatalf("invalid ID error=%v classification calls=%d", err, kind.calls)
	}
}

func TestCurrentGenerationReplanIsSiblingAuthorityAndCodingExcludesMemory(t *testing.T) {
	feedback := "Address the exact failed property."
	replan := &assemblyline.ObjectiveReplanAuthority{
		JobID: 904, Generation: 2, Feedback: feedback,
		FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
	}
	authority, err := newTurnAuthority(model.Job{
		ID: 904, Pipeline: model.PipelineChat, Instruction: "Original instruction.", CurrentGeneration: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := scriptedConversationCandidateProvider{
		replan: replan, memory: []assemblyline.MemoryContextCandidate{memoryContextCandidate(31, "private recall")},
	}
	selector := &scriptedConversationContextStation{memoryDecision: assemblyline.MemoryContextSelectionDecision{
		Schema: assemblyline.MemoryContextSelectionSchemaV1, ReferencedMemoryIDs: []int64{31},
	}}
	authority, _, err = resolveObjectiveMemoryContext(
		context.Background(), model.Job{ID: 904, CurrentGeneration: 2}, authority, provider, selector,
	)
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join(renderCodingObjectiveAuthority(authority), "\n")
	if !strings.Contains(rendered, feedback) || strings.Contains(rendered, "private recall") ||
		authority.Instruction != "Original instruction." {
		t.Fatalf("coding authority=%q instruction=%q", rendered, authority.Instruction)
	}

	provider.replan.JobID = 999
	if _, _, err := resolveObjectiveMemoryContext(
		context.Background(), model.Job{ID: 904, CurrentGeneration: 2}, authority, provider, selector,
	); err == nil {
		t.Fatal("cross-job replan authority was accepted")
	}
}

func TestObjectiveRepositoryReceivesUnifiedMemoryAndReplanContext(t *testing.T) {
	feedback := "Use the current generation's exact correction."
	provider := scriptedConversationCandidateProvider{
		memory: []assemblyline.MemoryContextCandidate{memoryContextCandidate(41, "exact repository recall")},
		replan: &assemblyline.ObjectiveReplanAuthority{
			JobID: 905, Generation: 2, Feedback: feedback,
			FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
		},
	}
	selector := &scriptedConversationContextStation{memoryDecision: assemblyline.MemoryContextSelectionDecision{
		Schema: assemblyline.MemoryContextSelectionSchemaV1, ReferencedMemoryIDs: []int64{41},
	}}
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1,
		Kind:   assemblyline.ObjectiveKindRepositoryRead,
	}}
	stations := &workflowRepositoryGroundingStation{}
	evidence := mustObjectiveEvidence(t, "R01", "Exact repository evidence.", "repository_symbol", "pack#symbol")
	_, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 905, Pipeline: model.PipelineChat, Instruction: "Explain the exact repository fact.", CurrentGeneration: 2,
	}, provider, selector, kind, &scriptedObjectiveConversationStation{}, stations,
		objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
			return objectiveRepositoryTestAcquisition([]objectiveEvidence{evidence}, 1), nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	for _, context := range []assemblyline.ObjectiveContext{
		kind.input.Context, stations.answerInput.Context, stations.reviewInputs[0].Context,
	} {
		if len(context.MemoryAuthorities) != 1 || context.MemoryAuthorities[0].MemoryID != 41 ||
			context.ReplanAuthority == nil || context.ReplanAuthority.Feedback != feedback {
			t.Fatalf("unified repository context=%+v", context)
		}
	}
}

func memoryContextCandidate(id int64, content string) assemblyline.MemoryContextCandidate {
	return assemblyline.MemoryContextCandidate{
		MemoryID: id, Kind: model.MemoryKindReference, Content: content,
		ContentSHA256: assemblyline.ExactObjectiveContextSHA(content),
	}
}
