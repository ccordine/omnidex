package contextcompiler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type scriptedTermsStation struct {
	decision assemblyline.ContextSearchTermsDecision
	calls    int
}

func (station *scriptedTermsStation) Generate(
	_ context.Context,
	input assemblyline.ContextSearchTermsInput,
) (assemblyline.ContextSearchTermsDecision, StationReceipt, error) {
	station.calls++
	if station.decision.Schema == "" {
		station.decision.Schema = assemblyline.ContextSearchTermsSchemaV1
	}
	return station.decision, StationReceipt{Calls: 1}, station.decision.ValidateFor(input)
}

type scriptedRelevanceStation struct {
	ids       []string
	idsByCall [][]string
	calls     int
	input     assemblyline.ContextRelevanceInput
	inputs    []assemblyline.ContextRelevanceInput
}

func (station *scriptedRelevanceStation) SelectRelevant(
	_ context.Context,
	input assemblyline.ContextRelevanceInput,
) (assemblyline.ContextRelevanceDecision, StationReceipt, error) {
	callIndex := station.calls
	station.calls++
	station.input = input
	station.inputs = append(station.inputs, input)
	ids := station.ids
	if callIndex < len(station.idsByCall) {
		ids = station.idsByCall[callIndex]
	}
	decision := assemblyline.ContextRelevanceDecision{
		Schema:                 assemblyline.ContextRelevanceSchemaV1,
		ReferencedCandidateIDs: append([]string{}, ids...),
	}
	return decision, StationReceipt{Calls: 1}, decision.ValidateFor(input)
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

func TestExplicitEmptyTermsReturnSelfContainedContextWithoutOptionalSieve(t *testing.T) {
	terms := &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
		Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{},
	}}
	relevance := &scriptedRelevanceStation{ids: []string{}}
	minifier := &scriptedMinificationStation{text: "must not run"}
	provider := &scriptedProvider{set: CandidateSet{}}

	result, err := Compile(t.Context(), Request{ExactInstruction: "Hello"}, provider, Stations{
		Terms: terms, Relevance: relevance, Minification: minifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if terms.calls != 1 || provider.calls != 1 || relevance.calls != 0 || minifier.calls != 0 {
		t.Fatalf("calls terms/provider/relevance/minifier=%d/%d/%d/%d", terms.calls, provider.calls, relevance.calls, minifier.calls)
	}
	if len(result.Context.Capsules) != 0 || result.ModelCalls != 1 ||
		result.RelevanceCalls != 0 || result.MinificationCalls != 0 {
		t.Fatalf("result=%#v", result)
	}
	if provider.gotTerms == nil || len(provider.gotTerms) != 0 {
		t.Fatalf("provider terms=%#v, want explicit empty set", provider.gotTerms)
	}
}

func TestCodeOwnedEmptyRetrievalDirectivePerformsNoSemanticCall(t *testing.T) {
	provider := &scriptedProvider{set: CandidateSet{}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction: "Hello.",
		Retrieval:        &RetrievalDirective{Concepts: []string{}},
	}, provider, Stations{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCalls != 0 || result.SearchTermsCalls != 0 ||
		result.RelevanceCalls != 0 || result.MinificationCalls != 0 {
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
		ExactInstruction: "Recall it.",
		Retrieval:        &RetrievalDirective{Concepts: []string{"Beacon", "beacon"}},
	}, provider, Stations{})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error=%v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("invalid directive reached acquisition %d times", provider.calls)
	}
}

func TestRequiredOptionalRelevanceRunsWithEmptySearchConcepts(t *testing.T) {
	optional := candidate(t, "simulation_inventory", "CTX_1", "Inventory item cloak: a rain-dark traveling cloak.")
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_1"}}
	provider := &scriptedProvider{set: CandidateSet{
		Optional: []assemblyline.ContextCandidateAuthority{optional},
	}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction: "I pull it tighter around my shoulders.",
	}, provider, Stations{
		Terms: &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
			Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{},
		}},
		Relevance: relevance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != 1 || len(relevance.input.RetrievalConcepts) != 0 ||
		result.ModelCalls != 2 || result.RelevanceCalls != 1 ||
		len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != optional.Content {
		t.Fatalf("empty-concept relevance result=%#v input=%#v", result, relevance.input)
	}
}

func TestAnaphoricTurnUsesSelectedAuthorityVerbatimWhenItFits(t *testing.T) {
	terms := &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
		Schema: assemblyline.ContextSearchTermsSchemaV1,
		Terms:  []string{"Previous Action", "immediately preceding rover maneuver"},
	}}
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_2"}}
	minifier := &scriptedMinificationStation{text: "must not run"}
	provider := &scriptedProvider{set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
		candidate(t, "conversation_user", "CTX_1", "The sample drawer was inventoried."),
		candidate(t, "conversation_assistant", "CTX_2", "I rotated the rover antenna toward Earth."),
	}}}

	result, err := Compile(t.Context(), Request{ExactInstruction: "Do it again."}, provider, Stations{
		Terms: terms, Relevance: relevance, Minification: minifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantConcepts := []string{"immediately preceding rover maneuver", "previous action"}
	if !reflect.DeepEqual(provider.gotTerms, wantConcepts) {
		t.Fatalf("provider terms=%#v", provider.gotTerms)
	}
	if !reflect.DeepEqual(relevance.input.RetrievalConcepts, wantConcepts) {
		t.Fatalf("relevance retrieval concepts=%#v", relevance.input.RetrievalConcepts)
	}
	if relevance.calls != 1 || minifier.calls != 0 {
		t.Fatalf("relevance/minification calls=%d/%d, want 1/0", relevance.calls, minifier.calls)
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
	responseJob, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Do it again.",
		Context: result.Context,
	})
	if err != nil {
		t.Fatal(err)
	}
	for label, job := range map[string]assemblyline.PortableJob{"final response": responseJob} {
		prompt, _, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		for _, concept := range wantConcepts {
			if strings.Contains(prompt, concept) {
				t.Fatalf("%s prompt leaked retrieval concept %q: %s", label, concept, prompt)
			}
		}
	}
}

func TestContextRelevancePagesAcquiredAuthoritiesWithoutGlobalCandidateWall(t *testing.T) {
	optional := make([]assemblyline.ContextCandidateAuthority, assemblyline.MaxContextCandidateAuthorities+1)
	for index := range optional {
		optional[index] = candidate(
			t, "conversation_exchange", fmt.Sprintf("CTX_%d", index+1),
			fmt.Sprintf("Distinct exchange %d.", index+1),
		)
	}
	relevance := &scriptedRelevanceStation{idsByCall: [][]string{{"CTX_1"}, {"CTX_17"}}}
	result, err := Compile(t.Context(), Request{ExactInstruction: "Recall the two exchanges."},
		&scriptedProvider{set: CandidateSet{Optional: optional}}, Stations{
			Terms: &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
				Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{"two exchanges"},
			}},
			Relevance: relevance,
		})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != 2 || len(relevance.inputs) != 2 ||
		len(relevance.inputs[0].CandidateAuthorities) != assemblyline.MaxContextCandidateAuthorities ||
		len(relevance.inputs[1].CandidateAuthorities) != 1 {
		t.Fatalf("paged relevance inputs=%#v", relevance.inputs)
	}
	if result.ModelCalls != 3 || result.RelevanceCalls != 2 || result.MinificationCalls != 0 {
		t.Fatalf("paged result=%#v", result)
	}
	if len(result.Context.Capsules) != 1 || len(result.Context.Capsules[0].Sources) != 2 ||
		result.Context.Capsules[0].Sources[0].CandidateID != "CTX_1" ||
		result.Context.Capsules[0].Sources[1].CandidateID != "CTX_17" {
		t.Fatalf("paged selection=%#v", result.Context)
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
		ExactInstruction: "Continue from the established relations.",
		Retrieval:        &RetrievalDirective{Concepts: []string{}},
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
	if result.ModelCalls != 3 || result.SearchTermsCalls != 0 ||
		result.MinificationCalls != 3 || len(result.Context.Capsules) != 1 ||
		result.Context.Capsules[0].Content != minifier.texts[2] ||
		len(result.Context.Capsules[0].Sources) != len(required) {
		t.Fatalf("hierarchical result=%#v", result)
	}
}

func TestProviderFailureAndInvalidSelectionFailWithoutRawContextFallback(t *testing.T) {
	want := errors.New("fixed provider failed")
	provider := &scriptedProvider{err: want}
	_, err := Compile(t.Context(), Request{ExactInstruction: "Recall the launch code."}, provider, Stations{
		Terms: &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
			Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{"launch code"},
		}}, Relevance: &scriptedRelevanceStation{}, Minification: &scriptedMinificationStation{},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want provider failure", err)
	}

	provider = &scriptedProvider{set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
		candidate(t, "durable_memory", "CTX_1", "The launch code is seven blue lanterns."),
	}}}
	_, err = Compile(t.Context(), Request{ExactInstruction: "Recall the launch code."}, provider, Stations{
		Terms: &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
			Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{"launch code"},
		}}, Relevance: &scriptedRelevanceStation{ids: []string{"CTX_99"}},
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
	result, err := Compile(t.Context(), Request{ExactInstruction: "Correct the answer."}, provider, Stations{
		Terms: &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
			Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{},
		}},
		Relevance:    relevance,
		Minification: minifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != 0 || minifier.calls != 0 || result.ModelCalls != 1 ||
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
	prompt, _, err := assemblyline.RenderPortableJob(projection)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, feedback) {
		t.Fatalf("response prompt lost verbatim replan context: %s", prompt)
	}
}

func TestReplanAuthorityWithoutRequiredExactCandidateFailsLoudly(t *testing.T) {
	feedback := "Use the corrected current value."
	_, err := Compile(t.Context(), Request{ExactInstruction: "Correct it."}, &scriptedProvider{set: CandidateSet{
		Replan: &assemblyline.ObjectiveReplanAuthority{
			JobID: 82, Generation: 2, Feedback: feedback,
			FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
		},
	}}, Stations{
		Terms: &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
			Schema: assemblyline.ContextSearchTermsSchemaV1, Terms: []string{},
		}},
		Relevance: &scriptedRelevanceStation{}, Minification: &scriptedMinificationStation{},
	})
	if err == nil {
		t.Fatal("replan authority bypassed its required exact context candidate")
	}
}
