package contextcompiler

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type pairwiseContextProvider struct {
	set CandidateSet
}

func (provider pairwiseContextProvider) SearchAvailability(context.Context) (SearchAvailability, error) {
	return SearchUnavailable, nil
}

func (provider pairwiseContextProvider) Retrieve(context.Context, []string) (CandidateSet, error) {
	return provider.set, nil
}

type pairwiseContextStation struct {
	rawByCandidate map[string]string
	inputs         []assemblyline.ContextRelevanceRelationInput
}

func (station *pairwiseContextStation) Relate(
	_ context.Context,
	input assemblyline.ContextRelevanceRelationInput,
) (assemblyline.ContextRelevanceRelationResult, StationReceipt, error) {
	station.inputs = append(station.inputs, input)
	result, err := assemblyline.DecodeContextRelevanceRelationResult(
		input, station.rawByCandidate[input.Candidate.CandidateID],
	)
	return result, StationReceipt{Calls: assemblyline.ExactSemanticLeafCalls}, err
}

func TestCompileExpandsCodeOwnedOptionalGroupAfterPairwiseRelevance(t *testing.T) {
	t.Parallel()
	authorities := pairwiseContextAuthorities(t)
	station := &pairwiseContextStation{rawByCandidate: map[string]string{
		"CTX_1": "B",
		"CTX_2": "A",
		"CTX_3": "B",
	}}
	result, err := Compile(
		context.Background(),
		pairwiseContextRequest(),
		pairwiseContextProvider{set: CandidateSet{
			Optional: authorities,
			OptionalSelectionGroups: []OptionalSelectionGroup{{
				CandidateIDs: []string{"CTX_2", "CTX_3"},
			}},
		}},
		Stations{Relevance: station},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(station.inputs) != len(authorities) {
		t.Fatalf("relevance leaves=%d, want %d", len(station.inputs), len(authorities))
	}
	for index, input := range station.inputs {
		if input.Candidate.CandidateID != authorities[index].CandidateID {
			t.Fatalf("leaf %d candidate=%q", index, input.Candidate.CandidateID)
		}
	}
	if result.RelevanceCalls != 3 || result.ModelCalls != 3 {
		t.Fatalf("call counts=%+v", result)
	}
	if len(result.Context.Capsules) != 1 {
		t.Fatalf("capsules=%d", len(result.Context.Capsules))
	}
	capsule := result.Context.Capsules[0]
	wantIDs := []string{"CTX_2", "CTX_3"}
	gotIDs := make([]string, len(capsule.Sources))
	for index, source := range capsule.Sources {
		gotIDs[index] = source.CandidateID
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("code-owned expanded IDs=%v, want %v", gotIDs, wantIDs)
	}
	if capsule.Content != "The inspection occurs Monday.\n\nThe paired calendar record confirms Monday." {
		t.Fatalf("compiled content=%q", capsule.Content)
	}
}

func TestCompileRejectsAggregateContextSelectionPackets(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`["CTX_1","CTX_2"]`,
		`{"candidate_ids":["CTX_1","CTX_2"]}`,
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			station := &pairwiseContextStation{rawByCandidate: map[string]string{
				"CTX_1": raw,
			}}
			_, err := Compile(
				context.Background(),
				pairwiseContextRequest(),
				pairwiseContextProvider{set: CandidateSet{
					Optional: pairwiseContextAuthorities(t)[:2],
				}},
				Stations{Relevance: station},
			)
			if err == nil {
				t.Fatal("aggregate model response was decoded as context selection")
			}
			if len(station.inputs) != 1 {
				t.Fatalf("relevance leaves=%d, want first binary leaf only", len(station.inputs))
			}
		})
	}
}

func pairwiseContextRequest() Request {
	return Request{
		ExactInstruction:   "Answer with the retained inspection date.",
		ModelInstruction:   "Answer with the retained inspection date.",
		Retrieval:          &RetrievalDirective{Availability: SearchUnavailable},
		KnownArtifactPaths: []string{},
	}
}

func pairwiseContextAuthorities(t *testing.T) []assemblyline.ContextCandidateAuthority {
	t.Helper()
	contents := []string{
		"The frame color is blue.",
		"The inspection occurs Monday.",
		"The paired calendar record confirms Monday.",
	}
	authorities := make([]assemblyline.ContextCandidateAuthority, len(contents))
	for index, content := range contents {
		candidate, err := assemblyline.NewContextCandidateAuthority(
			"repository", "CTX_"+string(rune('1'+index)), content,
		)
		if err != nil {
			t.Fatal(err)
		}
		authorities[index] = candidate
	}
	return authorities
}
