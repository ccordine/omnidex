package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type scriptedConversationCandidateProvider struct {
	candidates []assemblyline.ConversationContextTurn
	results    []assemblyline.ConversationSelectedAssistantResult
	memory     []assemblyline.MemoryContextCandidate
	replan     *assemblyline.ObjectiveReplanAuthority
	err        error
}

func (provider scriptedConversationCandidateProvider) MemoryCandidates(
	context.Context, model.Job,
) (objectiveMemoryContextCandidateSet, error) {
	return objectiveMemoryContextCandidateSet{
		Replan:     cloneObjectiveReplanAuthority(provider.replan),
		Candidates: append([]assemblyline.MemoryContextCandidate(nil), provider.memory...),
	}, provider.err
}

func (provider scriptedConversationCandidateProvider) Candidates(
	context.Context, model.Job,
) (conversationCandidateSet, error) {
	return conversationCandidateSet{
		Turns:            append([]assemblyline.ConversationContextTurn(nil), provider.candidates...),
		AssistantResults: append([]assemblyline.ConversationSelectedAssistantResult(nil), provider.results...),
	}, provider.err
}

type scriptedConversationContextStation struct {
	decision       assemblyline.ConversationContextSelectionDecision
	memoryDecision assemblyline.MemoryContextSelectionDecision
	calls          int
	memoryCalls    int
	input          assemblyline.ConversationContextSelectionInput
}

func (station *scriptedConversationContextStation) SelectMemory(
	_ context.Context,
	input assemblyline.MemoryContextSelectionInput,
) (assemblyline.MemoryContextSelectionDecision, objectiveStationReceipt, error) {
	station.memoryCalls++
	decision := station.memoryDecision
	if decision.Schema == "" {
		decision.Schema = assemblyline.MemoryContextSelectionSchemaV1
	}
	return decision, objectiveStationReceipt{Calls: 1}, nil
}

func (station *scriptedConversationContextStation) Select(
	_ context.Context,
	input assemblyline.ConversationContextSelectionInput,
) (assemblyline.ConversationContextSelectionDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	return station.decision, objectiveStationReceipt{Calls: 1}, nil
}

func TestConversationWithoutCandidateAuthoritiesMakesZeroContextSelectionCalls(t *testing.T) {
	selector := &scriptedConversationContextStation{}
	kind := answerObjectiveKindStation()
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 801, Pipeline: model.PipelineChat, Instruction: "Hello.",
	}, scriptedConversationCandidateProvider{}, selector, kind,
		&scriptedObjectiveConversationStation{}, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err != nil {
		t.Fatal(err)
	}
	if selector.calls != 0 || result.ModelCalls != 2 {
		t.Fatalf("selector calls=%d result=%#v", selector.calls, result)
	}
}

func TestConversationSelectsOnlyBoundedPriorUserAuthorityBeforeClassification(t *testing.T) {
	provider := scriptedConversationCandidateProvider{candidates: []assemblyline.ConversationContextTurn{
		{MessageID: 31, Role: assemblyline.ConversationContextUser, Content: "Compare the two cache implementations."},
		{
			MessageID: 32, Role: assemblyline.ConversationContextAssistant, PairedUserMessageID: 31,
			Content: "One favors reads; one favors writes.",
		},
	}}
	providerResults := []assemblyline.ConversationSelectedAssistantResult{{
		UserMessageID: 31, MessageID: 32, JobID: 71, Content: "One favors reads; one favors writes.",
	}}
	provider.results = providerResults
	selector := &scriptedConversationContextStation{decision: assemblyline.ConversationContextSelectionDecision{
		Schema: assemblyline.ConversationContextSelectionSchemaV1, ReferencedUserMessageIDs: []int64{31},
	}}
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 802, Pipeline: model.PipelineChat, Instruction: "Use the first one.",
	}, provider, selector, kind, conversation, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err != nil {
		t.Fatal(err)
	}
	want := []assemblyline.ConversationSelectedUserAuthority{{
		MessageID: 31, Content: "Compare the two cache implementations.",
	}}
	if !equalSelectedConversationAuthorities(kind.input.Context.UserAuthorities, want) {
		t.Fatalf("classifier selected authorities=%#v want %#v", kind.input.Context.UserAuthorities, want)
	}
	if !equalSelectedConversationAuthorities(conversation.input.Context.UserAuthorities, want) {
		t.Fatalf("response selected authorities=%#v want %#v", conversation.input.Context.UserAuthorities, want)
	}
	if len(conversation.input.Context.AssistantResults) != 1 ||
		conversation.input.Context.AssistantResults[0] != providerResults[0] {
		t.Fatalf("response paired results=%#v want %#v", conversation.input.Context.AssistantResults, providerResults)
	}
	if selector.calls != 1 || result.ModelCalls != 3 {
		t.Fatalf("selector calls=%d result=%#v", selector.calls, result)
	}
}

func TestInvalidConversationAuthorityReferenceFailsBeforeClassification(t *testing.T) {
	provider := scriptedConversationCandidateProvider{candidates: []assemblyline.ConversationContextTurn{{
		MessageID: 41, Role: assemblyline.ConversationContextUser, Content: "Inspect the parser.",
	}}}
	for _, invalidID := range []int64{40, 42, 999} {
		selector := &scriptedConversationContextStation{decision: assemblyline.ConversationContextSelectionDecision{
			Schema:                   assemblyline.ConversationContextSelectionSchemaV1,
			ReferencedUserMessageIDs: []int64{invalidID},
		}}
		kind := answerObjectiveKindStation()
		_, err := runObjectiveTurn(context.Background(), model.Job{
			ID: 803, Pipeline: model.PipelineChat, Instruction: "Fix that.",
		}, provider, selector, kind, &scriptedObjectiveConversationStation{},
			&scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
		if err == nil {
			t.Fatalf("invalid authority %d was accepted", invalidID)
		}
		if kind.calls != 0 {
			t.Fatalf("invalid authority %d reached classification %d times", invalidID, kind.calls)
		}
	}
}

func answerObjectiveKindStation() *scriptedObjectiveKindStation {
	return &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1,
		Kind:   assemblyline.ObjectiveKindAnswer,
	}}
}

func equalSelectedConversationAuthorities(
	left, right []assemblyline.ConversationSelectedUserAuthority,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
