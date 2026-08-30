package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/webresearch"
)

type scriptedObjectiveKindStation struct {
	decision assemblyline.ConversationObjectiveKindDecision
	calls    int
	input    assemblyline.ConversationObjectiveKindInput
	err      error
}

func TestObjectiveTurnProjectsKnownArtifactBeforeClassificationAndRestoresAcceptedAnswer(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{
		"internal/private/owner.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1,
		Kind:   assemblyline.ObjectiveKindAnswer,
	}}
	conversation := &scriptedObjectiveConversationStation{
		text: "ARTIFACT_1 owns the behavior.",
	}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 4101, Pipeline: model.PipelineChat,
		Instruction: "Explain owner.go.", Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind,
		conversation, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{
			ModelPathProvenance: provenance,
		})
	if err != nil {
		t.Fatal(err)
	}
	for label, value := range map[string]string{
		"kind":   kind.input.ExactInstruction,
		"answer": conversation.input.ExactInstruction,
	} {
		if strings.Contains(value, "owner.go") || value != "Explain ARTIFACT_1." {
			t.Fatalf("%s received unprojected instruction %q", label, value)
		}
	}
	if result.Output != "internal/private/owner.go owns the behavior." {
		t.Fatalf("restored answer=%q", result.Output)
	}
}

func (station *scriptedObjectiveKindStation) Classify(
	_ context.Context,
	input assemblyline.ConversationObjectiveKindInput,
) (assemblyline.ConversationObjectiveKindDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	return station.decision, objectiveStationReceipt{Calls: 1}, station.err
}

type scriptedObjectiveAnswerStation struct {
	calls int
	input assemblyline.GroundedAnswerInput
	err   error
}

type scriptedObjectiveConversationStation struct {
	calls int
	input assemblyline.ConversationResponseInput
	text  string
	err   error
}

func (station *scriptedObjectiveConversationStation) Respond(
	_ context.Context,
	input assemblyline.ConversationResponseInput,
	_ string,
) (assemblyline.ConversationResponseDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	text := station.text
	if text == "" {
		text = "A bounded answer."
	}
	return assemblyline.ConversationResponseDecision{
		Schema: assemblyline.ConversationResponseSchemaV1, Text: text,
	}, objectiveStationReceipt{Calls: 1}, station.err
}

func (station *scriptedObjectiveAnswerStation) Answer(
	_ context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	return assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: "A bounded answer.", EvidenceIDs: []string{input.Evidence[0].ID},
	}, objectiveStationReceipt{Calls: 1}, station.err
}

func TestObjectiveTurnPreservesExactAuthorityAndCodeOwnsCompletion(t *testing.T) {
	exact := "  Explain this behavior.  \n"
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindAnswer,
	}}
	conversation := &scriptedObjectiveConversationStation{}
	answer := &scriptedObjectiveAnswerStation{}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 41, Pipeline: model.PipelineChat, Instruction: exact, Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, conversation, answer, objectiveWorkflows{})
	if err != nil {
		t.Fatal(err)
	}
	if kind.input.ExactInstruction != exact {
		t.Fatalf("classifier instruction=%q want exact %q", kind.input.ExactInstruction, exact)
	}
	if conversation.input.ExactInstruction != exact {
		t.Fatalf("response instruction=%q want exact %q", conversation.input.ExactInstruction, exact)
	}
	if answer.calls != 0 {
		t.Fatalf("ungrounded answer invoked grounded station %d times", answer.calls)
	}
	if !result.Complete || result.Kind != assemblyline.ObjectiveKindAnswer || result.ModelCalls != 2 {
		t.Fatalf("result=%#v", result)
	}
	if result.ObjectiveID == "" || result.RequirementID == "" || result.Output != "A bounded answer." {
		t.Fatalf("result lacks code-owned identity/output: %#v", result)
	}
}

func TestObjectiveTurnMapsWorkspaceFactToCodeOwnedMutation(t *testing.T) {
	exact := "Change the implementation and run its tests."
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindWorkspaceMutation,
	}}
	answer := &scriptedObjectiveAnswerStation{}
	var got turnAuthority
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 42, Pipeline: model.PipelineChat, Instruction: exact, Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, &scriptedObjectiveConversationStation{}, answer, objectiveWorkflows{WorkspaceMutation: func(_ context.Context, authority turnAuthority) (string, error) {
		got = authority
		return "compiler and tests passed", nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Instruction != exact || got.JobID != 42 {
		t.Fatalf("mutation authority=%#v", got)
	}
	if answer.calls != 0 || !result.Complete || result.ModelCalls != 1 {
		t.Fatalf("answer calls=%d result=%#v", answer.calls, result)
	}
}

func TestObjectiveTurnConsumesOneCodeOwnedExternalWorkflowWithoutRestatingIt(t *testing.T) {
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindExternalAnswer,
	}}
	answer := &scriptedObjectiveAnswerStation{}
	evidence := mustObjectiveEvidence(t, "W01", "Authoritative current evidence.", "web_document", "https://example.test/current")
	evidence.ParagraphMask = 1
	rendered := "Current answer. [1]\n\nSources:\n[1] Current — https://example.test/current"
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 43, Pipeline: model.PipelineChat, Instruction: "What changed today?", Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, &scriptedObjectiveConversationStation{}, answer, objectiveWorkflows{ExternalAnswer: func(context.Context, turnAuthority) (objectiveExternalAnswer, error) {
		return objectiveExternalAnswer{
			Text: "Current answer.", Rendered: rendered,
			RenderedSHA256: objectiveTestSHA256(rendered),
			Paragraphs:     []webresearch.GroundedParagraph{{Text: "Current answer.", EvidenceIDs: []webresearch.EvidenceID{"W01"}}},
			Evidence:       []objectiveEvidence{evidence},
			EvidenceIDs:    []string{"W01"}, ModelCalls: 2,
			WebCallLedger: objectiveWebTestCallLedger(t, 2),
		}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if answer.calls != 0 {
		t.Fatalf("external synthesis was restated by grounded-answer station %d times", answer.calls)
	}
	if !result.Complete || result.Output != rendered || result.ModelCalls != 3 ||
		!reflect.DeepEqual(result.Citations, []objectiveEvidence{evidence}) {
		t.Fatalf("result=%#v", result)
	}
}

func TestObjectiveTurnFailsWithoutFallback(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindRepositoryRead,
	}}
	answer := &scriptedObjectiveAnswerStation{}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 44, Pipeline: model.PipelineChat, Instruction: "Explain this repository.", Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, &scriptedObjectiveConversationStation{}, answer, objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
		return objectiveEvidenceAcquisition{}, providerErr
	}})
	if !errors.Is(err, providerErr) {
		t.Fatalf("error=%v", err)
	}
	if result.Complete || answer.calls != 0 {
		t.Fatalf("failure fell through: result=%#v answer_calls=%d", result, answer.calls)
	}
}

func TestRetiredStoryTransportCannotReceiveTurnAuthority(t *testing.T) {
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindWorkspaceMutation,
	}}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 45, Pipeline: "story", Instruction: "Continue the scene.",
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, &scriptedObjectiveConversationStation{}, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{WorkspaceMutation: func(context.Context, turnAuthority) (string, error) {
		return "must not execute", nil
	}})
	if err == nil || result.Complete || kind.calls != 0 {
		t.Fatalf("retired story executed: result=%#v classifier=%d err=%v", result, kind.calls, err)
	}
}

func TestObjectiveStationCannotMutateCodeOwnedEvidenceBeforeAcceptance(t *testing.T) {
	evidence := []objectiveEvidence{mustObjectiveEvidence(t, "R01", "Frozen evidence.", "repository_symbol", "pack#symbol")}
	station := &mutatingObjectiveAnswerStation{}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 46, Pipeline: model.PipelineChat, Instruction: "Summarize the evidence.", Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindRepositoryRead,
	}}, &scriptedObjectiveConversationStation{}, station, objectiveWorkflows{RepositoryRead: func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error) {
		return objectiveRepositoryTestAcquisition(evidence, 1), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || evidence[0].Capsule.ID != "R01" || evidence[0].Capsule.Text != "Frozen evidence." {
		t.Fatalf("station changed machine evidence: evidence=%#v result=%#v", evidence, result)
	}
	if len(result.Citations) != 1 || result.Citations[0] != evidence[0] {
		t.Fatalf("accepted result lost exact cited evidence: result=%#v evidence=%#v", result, evidence)
	}
}

func TestObjectiveTurnRejectsInvalidEvidenceBeforeInference(t *testing.T) {
	valid := mustObjectiveEvidence(t, "W01", "Frozen evidence.", "web_document", "https://example.test/frozen")
	valid.SHA256 = "rewritten"
	valid.ParagraphMask = 1
	rendered := "Invalid answer. [1]\n\nSources:\n[1] Frozen — https://example.test/frozen"
	kind := &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindExternalAnswer,
	}}
	answer := &scriptedObjectiveAnswerStation{}
	_, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 47, Pipeline: model.PipelineChat, Instruction: "Summarize current evidence.", Metadata: objectiveAssistantMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, &scriptedObjectiveConversationStation{}, answer, objectiveWorkflows{ExternalAnswer: func(context.Context, turnAuthority) (objectiveExternalAnswer, error) {
		return objectiveExternalAnswer{
			Text: "Invalid answer.", Rendered: rendered, RenderedSHA256: objectiveTestSHA256(rendered),
			Paragraphs: []webresearch.GroundedParagraph{{Text: "Invalid answer.", EvidenceIDs: []webresearch.EvidenceID{"W01"}}},
			Evidence:   []objectiveEvidence{valid}, EvidenceIDs: []string{"W01"}, ModelCalls: 1,
			WebCallLedger: objectiveWebTestCallLedger(t, 1),
		}, nil
	}})
	if err == nil {
		t.Fatal("rewritten evidence projection reached inference")
	}
	if answer.calls != 0 {
		t.Fatalf("invalid code-owned evidence caused %d model calls", answer.calls)
	}
}

type mutatingObjectiveAnswerStation struct{}

func (*mutatingObjectiveAnswerStation) Answer(
	_ context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	input.Evidence[0].ID = "MUTATED"
	return assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: "Accepted from the frozen packet.", EvidenceIDs: []string{"R01"},
	}, objectiveStationReceipt{Calls: 1}, nil
}

func mustObjectiveEvidence(
	t *testing.T,
	id, text, sourceType, sourceRef string,
) objectiveEvidence {
	t.Helper()
	item, err := newObjectiveEvidence(id, text, sourceType, sourceRef)
	if err != nil {
		t.Fatal(err)
	}
	if sourceType == "web_document" {
		item.ObservedAt = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	} else if sourceType == "repository_symbol" || sourceType == "repository_relation" {
		item.SelectionText = text
	}
	return item
}
