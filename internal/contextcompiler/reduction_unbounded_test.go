package contextcompiler

import (
	"context"
	"strings"
	"testing"
)

func TestContextReductionDoesNotRecompactNecessaryOutputToAnArbitraryTarget(t *testing.T) {
	for _, fact := range []string{"The inspection requires this observation. ", "The display needs this locale detail. "} {
		selected := reductionCompletionAuthorities(t, 2, strings.Repeat(fact, 35))
		necessary := strings.TrimSpace(strings.Repeat(fact, 5000))
		station := &boundedReductionFixture{responses: []string{necessary}}
		result, err := Compile(context.Background(), pairwiseContextRequest(), pairwiseContextProvider{set: CandidateSet{Required: selected}}, Stations{Minification: station})
		if err != nil || result.ModelCalls != 1 || len(station.inputs) != 1 || len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != necessary {
			t.Fatalf("necessary context was squeezed, rejected or altered: calls=%d %v", len(station.inputs), err)
		}
	}
}
