package contextcompiler

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type boundedReductionFixture struct {
	responses []string
	inputs    []assemblyline.ContextMinificationInput
}

func (fixture *boundedReductionFixture) Minify(_ context.Context, input assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
	fixture.inputs = append(fixture.inputs, input)
	index := len(fixture.inputs) - 1
	if index >= len(fixture.responses) {
		return assemblyline.ContextMinificationDecision{}, 0, fmt.Errorf("unnecessary context reduction reached inference")
	}
	decision, err := assemblyline.DecodeContextMinificationDecision(input, fixture.responses[index])
	return decision, 1, err
}

func TestContextReductionStopsWhenRemainingExactContentFits(t *testing.T) {
	for _, fixture := range []struct{ repeated, reduced, tail string }{
		{"The inspection takes place on Tuesday. ", "The inspection is Tuesday.", "The venue is the north hall."},
		{"The preferred display language is French. ", "Use French for display text.", "Retain the submitted punctuation."},
	} {
		t.Run(fixture.reduced, func(t *testing.T) {
			selected := reductionCompletionAuthorities(t, 8, strings.Repeat(fixture.repeated, 8))
			selected = append(selected, reductionCompletionAuthority(t, 9, fixture.tail), reductionCompletionAuthority(t, 10, "This statement remains unchanged."))
			before := append([]assemblyline.ContextCandidateAuthority(nil), selected...)
			station := &boundedReductionFixture{responses: []string{fixture.reduced}}
			result, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: CandidateSet{Required: selected}}, Stations{Minification: station})
			if err != nil {
				t.Fatal(err)
			}
			if result.ModelCalls != 1 || len(station.inputs) != 1 {
				t.Fatalf("extra semantic work: %+v calls=%d", result, len(station.inputs))
			}
			if !reflect.DeepEqual(selected, before) || !reflect.DeepEqual(station.inputs[0].SelectedAuthorities, selected[:8]) {
				t.Fatal("reduction changed original source values or included later groups")
			}
			want := strings.Join([]string{fixture.reduced, fixture.tail, "This statement remains unchanged."}, "\n\n")
			if len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != want || len(result.Context.Capsules[0].Sources) != len(selected) {
				t.Fatalf("remaining context was not retained exactly: %+v", result.Context)
			}
		})
	}
}

func TestContextReductionContinuesOnlyWhileTheCombinedContentExceedsItsBound(t *testing.T) {
	selected := reductionCompletionAuthorities(t, 12, strings.Repeat("An observed value is retained. ", 22))
	station := &boundedReductionFixture{responses: []string{strings.Repeat("Retained detail. ", 70) + "End.", "The remaining value is settled."}}
	result, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: CandidateSet{Required: selected}}, Stations{Minification: station})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCalls != 2 || len(station.inputs) != 2 || !reflect.DeepEqual(station.inputs[0].SelectedAuthorities, selected[:8]) || !reflect.DeepEqual(station.inputs[1].SelectedAuthorities, selected[8:]) {
		t.Fatalf("required reductions were skipped or widened: %+v %v", result, station.inputs)
	}
	if result.Context.Capsules[0].Content != strings.Join(station.responses, "\n\n") {
		t.Fatalf("wrong reduced content: %+v", result.Context)
	}
}

func TestContextReductionStillFailsOnInvalidRequiredSemanticOutput(t *testing.T) {
	selected := reductionCompletionAuthorities(t, 10, strings.Repeat("The current observation is known. ", 9))
	station := &boundedReductionFixture{responses: []string{""}}
	if _, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: CandidateSet{Required: selected}}, Stations{Minification: station}); err == nil || len(station.inputs) != 1 {
		t.Fatalf("invalid minification continued: err=%v calls=%d", err, len(station.inputs))
	}
}

func reductionCompletionAuthorities(t *testing.T, count int, content string) []assemblyline.ContextCandidateAuthority {
	t.Helper()
	selected := make([]assemblyline.ContextCandidateAuthority, count)
	for index := range selected {
		selected[index] = reductionCompletionAuthority(t, index+1, content+fmt.Sprintf("Observation %d.", index+1))
	}
	return selected
}

func reductionCompletionAuthority(t *testing.T, ordinal int, content string) assemblyline.ContextCandidateAuthority {
	t.Helper()
	value, err := assemblyline.NewContextCandidateAuthority("repository", fmt.Sprintf("CTX_%d", ordinal), content)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
