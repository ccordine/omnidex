package contextcompiler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type scriptedRelevanceStation struct {
	ids          []string
	receiptCalls int
	calls        int
	invalidHash  bool
	input        assemblyline.ContextRelevanceRelationInput
	inputs       []assemblyline.ContextRelevanceRelationInput
}

func (station *scriptedRelevanceStation) Relate(
	_ context.Context,
	input assemblyline.ContextRelevanceRelationInput,
) (assemblyline.ContextRelevanceRelationResult, StationReceipt, error) {
	station.calls++
	station.input = input
	station.inputs = append(station.inputs, input)
	raw := assemblyline.ContextCandidateNotDirectlyRelevant
	for _, candidateID := range station.ids {
		if input.Candidate.CandidateID == candidateID {
			raw = assemblyline.ContextCandidateDirectlyRelevant
			break
		}
	}
	decision, err := assemblyline.DecodeContextRelevanceRelationResult(input, raw)
	if err != nil {
		return assemblyline.ContextRelevanceRelationResult{}, StationReceipt{}, err
	}
	if station.invalidHash {
		decision.AuthoritySHA256 = strings.Repeat("0", 64)
	}
	receiptCalls := station.receiptCalls
	if receiptCalls == 0 {
		receiptCalls = 1
	}
	return decision, StationReceipt{Calls: receiptCalls}, nil
}

type scriptedMinificationStation struct {
	text   string
	texts  []string
	calls  int
	input  assemblyline.ContextMinificationInput
	inputs []assemblyline.ContextMinificationInput
}

func (station *scriptedMinificationStation) Minify(
	_ context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, StationReceipt, error) {
	callIndex := station.calls
	station.calls++
	station.input = input
	station.inputs = append(station.inputs, input)
	text := station.text
	if callIndex < len(station.texts) {
		text = station.texts[callIndex]
	}
	decision := assemblyline.ContextMinificationDecision{
		Schema: assemblyline.ContextMinificationSchemaV1, MinimalContext: text,
	}
	return decision, StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

type scriptedProvider struct {
	set               CandidateSet
	err               error
	availability      SearchAvailability
	availabilityError error
	availabilityCalls int
	calls             int
	gotTerms          []string
}

func (provider *scriptedProvider) SearchAvailability(
	_ context.Context,
) (SearchAvailability, error) {
	provider.availabilityCalls++
	if provider.availability == "" {
		return SearchAvailable, provider.availabilityError
	}
	return provider.availability, provider.availabilityError
}

func (provider *scriptedProvider) Retrieve(
	_ context.Context,
	terms []string,
) (CandidateSet, error) {
	provider.calls++
	provider.gotTerms = append([]string{}, terms...)
	return provider.set, provider.err
}

func candidate(t *testing.T, namespace, id, content string) assemblyline.ContextCandidateAuthority {
	t.Helper()
	value, err := assemblyline.NewContextCandidateAuthority(namespace, id, content)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func contextRequest(exactInstruction string) Request {
	return Request{
		ExactInstruction: exactInstruction, ModelInstruction: exactInstruction,
		KnownArtifactPaths: []string{},
	}
}

func TestAvailableRetrievalUsesExactInstructionWithoutSemanticQuery(t *testing.T) {
	relevance := &scriptedRelevanceStation{ids: []string{}}
	minifier := &scriptedMinificationStation{text: "must not run"}
	provider := &scriptedProvider{set: CandidateSet{}}

	result, err := Compile(t.Context(), contextRequest("Hello"), provider, Stations{
		Relevance: relevance, Minification: minifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || relevance.calls != 0 || minifier.calls != 0 {
		t.Fatalf("calls provider/relevance/minifier=%d/%d/%d", provider.calls, relevance.calls, minifier.calls)
	}
	if len(result.Context.Capsules) != 0 || result.ModelCalls != 0 ||
		result.RelevanceCalls != 0 || result.MinificationCalls != 0 {
		t.Fatalf("result=%#v", result)
	}
	if len(provider.gotTerms) != 1 || provider.gotTerms[0] != "Hello" {
		t.Fatalf("provider queries=%#v, want exact instruction", provider.gotTerms)
	}
}

func TestContextCompilerSeparatesRawRetrievalFromPathFreeModelAuthority(t *testing.T) {
	t.Parallel()
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_1"}}
	provider := &scriptedProvider{set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
		candidate(t, "conversation", "CTX_1", "The earlier exchange mentions secret_owner.go."),
	}}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Inspect internal/private/secret_owner.go again.",
		ModelInstruction:   "Inspect ARTIFACT_1 again.",
		KnownArtifactPaths: []string{"internal/private/secret_owner.go"},
	}, provider, Stations{Relevance: relevance})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.gotTerms) != 1 || provider.gotTerms[0] != "Inspect internal/private/secret_owner.go again." {
		t.Fatalf("deterministic retrieval lost raw authority: %#v", provider.gotTerms)
	}
	if relevance.input.ExactInstruction != "Inspect ARTIFACT_1 again." ||
		len(relevance.input.KnownArtifactPaths) != 1 ||
		relevance.input.KnownArtifactPaths[0] != "internal/private/secret_owner.go" {
		t.Fatalf("context relevance received wrong model/provenance authority: %#v", relevance.input)
	}
	if len(result.Context.Capsules) != 1 {
		t.Fatalf("compiled context=%#v", result.Context)
	}
}

func TestCodeOwnedEmptyRetrievalDirectivePerformsNoSemanticCall(t *testing.T) {
	provider := &scriptedProvider{set: CandidateSet{}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Hello.",
		ModelInstruction:   "Hello.",
		KnownArtifactPaths: []string{},
		Retrieval: &RetrievalDirective{
			Availability: SearchUnavailable,
		},
	}, provider, Stations{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCalls != 0 || result.RelevanceCalls != 0 || result.MinificationCalls != 0 {
		t.Fatalf("ceremonial semantic work was performed: %#v", result)
	}
	if provider.calls != 1 || provider.gotTerms == nil || len(provider.gotTerms) != 0 {
		t.Fatalf("provider calls/terms=%d/%#v", provider.calls, provider.gotTerms)
	}
	if provider.availabilityCalls != 0 {
		t.Fatalf("code-owned retrieval unexpectedly rechecked availability %d times", provider.availabilityCalls)
	}
}

func TestInvalidCodeOwnedRetrievalDirectiveFailsBeforeAcquisition(t *testing.T) {
	provider := &scriptedProvider{}
	_, err := Compile(t.Context(), Request{
		ExactInstruction:   "Recall it.",
		ModelInstruction:   "Recall it.",
		KnownArtifactPaths: []string{},
		Retrieval: &RetrievalDirective{
			Availability: SearchAvailability("unknown"),
		},
	}, provider, Stations{})
	if err == nil || !strings.Contains(err.Error(), "availability") {
		t.Fatalf("error=%v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("invalid directive reached acquisition %d times", provider.calls)
	}
}

func TestRequiredOptionalRelevanceRunsAgainstExactInstruction(t *testing.T) {
	optional := candidate(t, "simulation_inventory", "CTX_1", "Inventory item cloak: a rain-dark traveling cloak.")
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_1"}}
	provider := &scriptedProvider{set: CandidateSet{
		Optional: []assemblyline.ContextCandidateAuthority{optional},
	}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "I pull it tighter around my shoulders.",
		ModelInstruction:   "I pull it tighter around my shoulders.",
		KnownArtifactPaths: []string{},
	}, provider, Stations{Relevance: relevance})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != 1 || result.ModelCalls != 1 || result.RelevanceCalls != 1 ||
		len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != optional.Content {
		t.Fatalf("empty-concept relevance result=%#v input=%#v", result, relevance.input)
	}
}

func TestContextReceiptRejectsRetryMultiplierForOneSemanticLeaf(t *testing.T) {
	t.Parallel()
	optional := candidate(t, "fictional_canon", "CTX_1", "The gate remains sealed.")
	relevance := &scriptedRelevanceStation{
		ids: []string{"CTX_1"}, receiptCalls: 3,
	}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Continue from the sealed gate.",
		ModelInstruction:   "Continue from the sealed gate.",
		KnownArtifactPaths: []string{},
		Retrieval: &RetrievalDirective{
			Availability: SearchUnavailable,
		},
	}, &scriptedProvider{set: CandidateSet{
		Optional: []assemblyline.ContextCandidateAuthority{optional},
	}}, Stations{Relevance: relevance})
	if err == nil || result.ModelCalls != 0 || result.RelevanceCalls != 0 {
		t.Fatalf("retry-multiplied context result=%#v error=%v", result, err)
	}
}

func TestContextRelevanceReceiptAcceptsMultipleBoundedLeaves(t *testing.T) {
	t.Parallel()
	optional := []assemblyline.ContextCandidateAuthority{
		candidate(t, "fictional_canon", "CTX_1", "The gate remains sealed."),
		candidate(t, "fictional_canon", "CTX_2", "The warning bell is ringing."),
		candidate(t, "fictional_canon", "CTX_3", "The eastern path is flooded."),
		candidate(t, "fictional_canon", "CTX_4", "The keeper carries the brass key."),
	}
	relevance := &scriptedRelevanceStation{
		ids: []string{"CTX_1", "CTX_2", "CTX_3", "CTX_4"},
	}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Continue from the established scene.",
		ModelInstruction:   "Continue from the established scene.",
		KnownArtifactPaths: []string{},
		Retrieval: &RetrievalDirective{
			Availability: SearchUnavailable,
		},
	}, &scriptedProvider{set: CandidateSet{Optional: optional}}, Stations{
		Relevance: relevance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RelevanceCalls != 4 || result.ModelCalls != 4 ||
		len(result.Context.Capsules) != 1 ||
		len(result.Context.Capsules[0].Sources) != len(optional) {
		t.Fatalf("multi-leaf context result=%#v", result)
	}
}

func TestAnaphoricTurnUsesSelectedAuthorityVerbatimWhenItFits(t *testing.T) {
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_2"}}
	minifier := &scriptedMinificationStation{text: "must not run"}
	provider := &scriptedProvider{set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
		candidate(t, "conversation_user", "CTX_1", "The sample drawer was inventoried."),
		candidate(t, "conversation_assistant", "CTX_2", "I rotated the rover antenna toward Earth."),
	}}}

	result, err := Compile(t.Context(), contextRequest("Do it again."), provider, Stations{
		Relevance: relevance, Minification: minifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.gotTerms) != 1 || provider.gotTerms[0] != "Do it again." {
		t.Fatalf("provider queries=%#v", provider.gotTerms)
	}
	if relevance.calls != 2 || minifier.calls != 0 {
		t.Fatalf("relevance/minification calls=%d/%d, want 2/0", relevance.calls, minifier.calls)
	}
	if len(result.Context.Capsules) != 1 ||
		result.Context.Capsules[0].Content != "I rotated the rover antenna toward Earth." ||
		len(result.Context.Capsules[0].Sources) != 1 ||
		result.Context.Capsules[0].Sources[0].CandidateID != "CTX_2" ||
		result.ModelCalls != 2 {
		t.Fatalf("result=%#v", result)
	}
	if err := result.Context.Validate(); err != nil {
		t.Fatalf("compiled context invalid: %v", err)
	}
}

func TestContextRelevanceRelatesEveryAcquiredAuthorityOnceWithoutGlobalCandidateWall(t *testing.T) {
	optional := make([]assemblyline.ContextCandidateAuthority, assemblyline.MaxContextCandidateAuthorities+1)
	for index := range optional {
		optional[index] = candidate(
			t, "conversation_exchange", fmt.Sprintf("CTX_%d", index+1),
			fmt.Sprintf("Distinct exchange %d.", index+1),
		)
	}
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_1", "CTX_17"}}
	result, err := Compile(t.Context(), contextRequest("Recall the two exchanges."),
		&scriptedProvider{set: CandidateSet{Optional: optional}}, Stations{Relevance: relevance})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != len(optional) || len(relevance.inputs) != len(optional) {
		t.Fatalf("per-candidate relevance inputs=%#v", relevance.inputs)
	}
	for index, input := range relevance.inputs {
		if input.Candidate.CandidateID != optional[index].CandidateID {
			t.Fatalf("candidate %d input=%#v", index, input)
		}
	}
	if result.ModelCalls != len(optional) || result.RelevanceCalls != len(optional) ||
		result.MinificationCalls != 0 {
		t.Fatalf("per-candidate result=%#v", result)
	}
	if len(result.Context.Capsules) != 1 || len(result.Context.Capsules[0].Sources) != 2 ||
		result.Context.Capsules[0].Sources[0].CandidateID != "CTX_1" ||
		result.Context.Capsules[0].Sources[1].CandidateID != "CTX_17" {
		t.Fatalf("per-candidate selection=%#v", result.Context)
	}
}

func TestSelectedContextUsesHierarchicalMinificationOnlyAfterVerbatimDoesNotFit(t *testing.T) {
	required := make([]assemblyline.ContextCandidateAuthority, 6)
	for index := range required {
		required[index] = candidate(
			t, "fictional_canon", fmt.Sprintf("CTX_%d", index+1),
			fmt.Sprintf("Authority %d: %s", index+1, strings.Repeat(string(rune('a'+index)), 1_780)),
		)
	}
	minifier := &scriptedMinificationStation{texts: []string{
		"First retained relation: " + strings.Repeat("x", 1_150),
		"Second retained relation: " + strings.Repeat("y", 1_150),
		"Final retained context preserves the required relations.",
	}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Continue from the established relations.",
		ModelInstruction:   "Continue from the established relations.",
		KnownArtifactPaths: []string{},
		Retrieval: &RetrievalDirective{
			Availability: SearchUnavailable,
		},
	}, &scriptedProvider{set: CandidateSet{Required: required}}, Stations{Minification: minifier})
	if err != nil {
		t.Fatal(err)
	}
	if minifier.calls != 3 || len(minifier.inputs) != 3 {
		t.Fatalf("minification calls/inputs=%d/%d", minifier.calls, len(minifier.inputs))
	}
	if len(minifier.inputs[0].SelectedAuthorities) != 3 ||
		len(minifier.inputs[1].SelectedAuthorities) != 3 ||
		len(minifier.inputs[2].SelectedAuthorities) != 2 {
		t.Fatalf("hierarchical groups=%d/%d/%d",
			len(minifier.inputs[0].SelectedAuthorities),
			len(minifier.inputs[1].SelectedAuthorities),
			len(minifier.inputs[2].SelectedAuthorities),
		)
	}
	if result.ModelCalls != 3 || result.MinificationCalls != 3 || len(result.Context.Capsules) != 1 ||
		result.Context.Capsules[0].Content != minifier.texts[2] ||
		len(result.Context.Capsules[0].Sources) != len(required) {
		t.Fatalf("hierarchical result=%#v", result)
	}
}

func TestProviderFailureAndInvalidSelectionFailWithoutRawContextFallback(t *testing.T) {
	want := errors.New("fixed provider failed")
	provider := &scriptedProvider{err: want}
	_, err := Compile(t.Context(), contextRequest("Recall the launch code."), provider, Stations{
		Relevance: &scriptedRelevanceStation{}, Minification: &scriptedMinificationStation{},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want provider failure", err)
	}

	provider = &scriptedProvider{set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
		candidate(t, "durable_memory", "CTX_1", "The launch code is seven blue lanterns."),
	}}}
	_, err = Compile(t.Context(), contextRequest("Recall the launch code."), provider, Stations{
		Relevance:    &scriptedRelevanceStation{ids: []string{"CTX_1"}, invalidHash: true},
		Minification: &scriptedMinificationStation{text: "unreachable"},
	})
	if err == nil {
		t.Fatal("unknown relevance ID was accepted")
	}
}

func TestReplanFeedbackIsPreservedVerbatimWhenItFits(t *testing.T) {
	feedback := "Replace the earlier answer with the corrected orbital period and omit the unrelated launch history."
	replan := &assemblyline.ObjectiveReplanAuthority{
		JobID: 81, Generation: 2, Feedback: feedback,
		FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
	}
	minifier := &scriptedMinificationStation{text: "must not run"}
	provider := &scriptedProvider{set: CandidateSet{
		Required: []assemblyline.ContextCandidateAuthority{
			candidate(t, "objective_replan", "CTX_1", feedback),
		},
		Replan: replan,
	}}
	relevance := &scriptedRelevanceStation{ids: []string{}}
	result, err := Compile(t.Context(), contextRequest("Correct the answer."), provider, Stations{
		Relevance:    relevance,
		Minification: minifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != 0 || minifier.calls != 0 || result.ModelCalls != 0 ||
		result.RelevanceCalls != 0 || result.MinificationCalls != 0 {
		t.Fatalf("replan sieve calls relevance/minification=%d/%d result=%#v", relevance.calls, minifier.calls, result)
	}
	if len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != feedback ||
		result.Context.ReplanAuthority == nil || result.Context.ReplanAuthority.Feedback != feedback {
		t.Fatalf("compiled replan context=%#v", result.Context)
	}
	projection, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Correct the answer.",
		Context: result.Context,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(projection)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, feedback) {
		t.Fatalf("response prompt lost verbatim replan context: %s", prompt)
	}
}

func TestReplanAuthorityWithoutRequiredExactCandidateFailsLoudly(t *testing.T) {
	feedback := "Use the corrected current value."
	_, err := Compile(t.Context(), contextRequest("Correct it."), &scriptedProvider{set: CandidateSet{
		Replan: &assemblyline.ObjectiveReplanAuthority{
			JobID: 82, Generation: 2, Feedback: feedback,
			FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
		},
	}}, Stations{
		Relevance: &scriptedRelevanceStation{}, Minification: &scriptedMinificationStation{},
	})
	if err == nil {
		t.Fatal("replan authority bypassed its required exact context candidate")
	}
}

func TestContextStationReceiptAcceptsOnlyZeroCallDurableReuse(t *testing.T) {
	t.Parallel()
	if err := validateReceipt("context relevance", StationReceipt{Reused: true}); err != nil {
		t.Fatal(err)
	}
	if err := validateReceipt(
		"context relevance", StationReceipt{Calls: 1, Reused: true},
	); err == nil {
		t.Fatal("context reuse fabricated a provider call")
	}
	if err := validateReceipt("context relevance", StationReceipt{}); err == nil {
		t.Fatal("zero-call context station lacked durable reuse provenance")
	}
}
