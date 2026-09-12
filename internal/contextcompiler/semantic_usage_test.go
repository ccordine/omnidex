package contextcompiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestContextSemanticValuesNeedNoPositiveCallReceipt(t *testing.T) {
	for _, text := range []string{"The inspection is on Tuesday.", "The preferred display language is French."} {
		for _, calls := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/%d", text, calls), func(t *testing.T) {
				selected := reductionCompletionAuthorities(t, 2, strings.Repeat(text+" ", 45))
				stations := contextUsageStations(calls, text)
				result, err := Compile(context.Background(), pairwiseContextRequest(),
					pairwiseContextProvider{set: CandidateSet{Optional: selected}}, stations)
				if err != nil || result.ModelCalls != 3*calls || len(result.Context.Capsules) != 1 {
					t.Fatalf("valid values were blocked by call metadata: %+v / %v", result, err)
				}
				wantSources := []assemblyline.ObjectiveContextSource{
					{Namespace: selected[0].Namespace, CandidateID: selected[0].CandidateID},
					{Namespace: selected[1].Namespace, CandidateID: selected[1].CandidateID},
				}
				if result.Context.Capsules[0].Content != text || !reflect.DeepEqual(result.Context.Capsules[0].Sources, wantSources) {
					t.Fatalf("context did not consume actual selected values: %+v", result.Context)
				}
			})
		}
	}
}

func TestContextSemanticFailuresRetainActualUsage(t *testing.T) {
	for _, phase := range []string{"relevance", "minification"} {
		t.Run(phase, func(t *testing.T) {
			failure := errors.New("provider returned an incomplete response")
			stations := contextUsageStations(1, "The observed value is retained.")
			invoked := 0
			if phase == "relevance" {
				stations.Relevance = contextUsageRelevanceFunc(func(input assemblyline.ContextRelevanceRelationInput) (assemblyline.ContextRelevanceRelationResult, int, error) {
					invoked++
					if invoked == 2 {
						return assemblyline.ContextRelevanceRelationResult{}, 1, failure
					}
					value, err := assemblyline.DecodeContextRelevanceRelationResult(input, "A")
					return value, 1, err
				})
			} else {
				stations.Minification = contextUsageMinificationFunc(func(assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
					invoked++
					return assemblyline.ContextMinificationDecision{}, 1, failure
				})
			}
			selected := reductionCompletionAuthorities(t, 2, strings.Repeat("The observed value is retained. ", 45))
			result, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: CandidateSet{Optional: selected}}, stations)
			wantCalls, wantInvoked := 2, 2
			if phase == "minification" {
				wantCalls, wantInvoked = 3, 1
			}
			if !errors.Is(err, failure) || result.ModelCalls != wantCalls || len(result.Context.Capsules) != 0 || invoked != wantInvoked {
				t.Fatalf("failure lost usage or accepted partial context: %+v invoked=%d / %v", result, invoked, err)
			}
		})
	}
}

func TestContextZeroCallsDoNotAuthorizeInvalidValues(t *testing.T) {
	for _, phase := range []string{"relevance", "minification"} {
		t.Run(phase, func(t *testing.T) {
			stations := contextUsageStations(0, "The inspection is Tuesday.")
			wantError := "relation schema"
			if phase == "relevance" {
				stations.Relevance = contextUsageRelevanceFunc(func(assemblyline.ContextRelevanceRelationInput) (assemblyline.ContextRelevanceRelationResult, int, error) {
					return assemblyline.ContextRelevanceRelationResult{}, 0, nil
				})
			} else {
				wantError = "minification schema"
				stations.Minification = contextUsageMinificationFunc(func(assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
					return assemblyline.ContextMinificationDecision{}, 0, nil
				})
			}
			selected := reductionCompletionAuthorities(t, 2, strings.Repeat("The observed value is retained. ", 45))
			result, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: CandidateSet{Optional: selected}}, stations)
			if err == nil || !strings.Contains(err.Error(), wantError) || len(result.Context.Capsules) != 0 || result.ModelCalls != 0 {
				t.Fatalf("invalid value did not fail its own validation: %+v / %v", result, err)
			}
		})
	}
}

func TestContextSemanticCallBoundsRemainEnforced(t *testing.T) {
	for _, calls := range []int{-1, 2} {
		for _, phase := range []string{"relevance", "minification"} {
			t.Run(fmt.Sprintf("%s/%d", phase, calls), func(t *testing.T) {
				stations := contextUsageStations(calls, "The observed value is retained.")
				selected := reductionCompletionAuthorities(t, 2, strings.Repeat("The observed value is retained. ", 45))
				set := CandidateSet{Optional: selected}
				if phase == "minification" {
					set = CandidateSet{Required: selected}
				}
				result, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: set}, stations)
				if err == nil || !strings.Contains(err.Error(), "outside 0..1") || len(result.Context.Capsules) != 0 {
					t.Fatalf("invalid usage accepted context: %+v / %v", result, err)
				}
			})
		}
	}
}

func TestContextCallReceiptAndReuseGateAreRemoved(t *testing.T) {
	for _, path := range []string{"types.go", "compiler.go", "reduction.go", "../worker/objective_context_sieve_stations.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"StationReceipt", "validateReceipt", ".Reused", "RelevanceCalls", "MinificationCalls"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains removed context call-proof machinery %q", path, forbidden)
			}
		}
	}
}

func contextUsageStations(calls int, summary string) Stations {
	return Stations{
		Relevance: contextUsageRelevanceFunc(func(input assemblyline.ContextRelevanceRelationInput) (assemblyline.ContextRelevanceRelationResult, int, error) {
			value, err := assemblyline.DecodeContextRelevanceRelationResult(input, "A")
			return value, calls, err
		}),
		Minification: contextUsageMinificationFunc(func(input assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
			value, err := assemblyline.DecodeContextMinificationDecision(input, summary)
			return value, calls, err
		}),
	}
}

type contextUsageRelevanceFunc func(assemblyline.ContextRelevanceRelationInput) (assemblyline.ContextRelevanceRelationResult, int, error)

func (resolve contextUsageRelevanceFunc) Relate(_ context.Context, input assemblyline.ContextRelevanceRelationInput) (assemblyline.ContextRelevanceRelationResult, int, error) {
	return resolve(input)
}

type contextUsageMinificationFunc func(assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error)

func (resolve contextUsageMinificationFunc) Minify(_ context.Context, input assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
	return resolve(input)
}
