package contextcompiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestOptionalSelectionGroupExpandsOneRelevantMemberBeforeMinificationAndProvenance(t *testing.T) {
	optional := make([]assemblyline.ContextCandidateAuthority, 17)
	for index := 0; index < 14; index++ {
		optional[index] = candidate(
			t, "conversation_exchange", fmt.Sprintf("CTX_%d", index+1),
			fmt.Sprintf("Unrelated exchange %d.", index+1),
		)
	}
	for index, fill := range []string{"a", "b", "c"} {
		candidateIndex := index + 15
		optional[candidateIndex-1] = candidate(
			t, "conversation_exchange", fmt.Sprintf("CTX_%d", candidateIndex),
			fmt.Sprintf("Complete exchange segment %d: %s", index+1, strings.Repeat(fill, 850)),
		)
	}
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_17"}}
	minifier := &scriptedMinificationStation{text: "The complete grouped exchange remains authoritative."}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Use the marker in the final exchange segment.",
		ModelInstruction:   "Use the marker in the final exchange segment.",
		KnownArtifactPaths: []string{},
		Retrieval: &RetrievalDirective{
			Availability: SearchAvailable,
		},
	}, &scriptedProvider{set: CandidateSet{
		Optional: optional,
		OptionalSelectionGroups: []OptionalSelectionGroup{{
			CandidateIDs: []string{"CTX_15", "CTX_16", "CTX_17"},
		}},
	}}, Stations{Relevance: relevance, Minification: minifier})
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != len(optional) || len(relevance.inputs) != len(optional) {
		t.Fatalf("per-candidate relevance=%#v", relevance.inputs)
	}
	if minifier.calls != 1 || len(minifier.inputs) != 1 ||
		!candidateIDsEqual(
			minifier.inputs[0].SelectedAuthorities,
			[]string{"CTX_15", "CTX_16", "CTX_17"},
		) {
		t.Fatalf("grouped minification inputs=%#v", minifier.inputs)
	}
	if result.RelevanceCalls != len(optional) || result.MinificationCalls != 1 ||
		result.ModelCalls != len(optional)+1 ||
		len(result.Context.Capsules) != 1 ||
		result.Context.Capsules[0].Content != minifier.text ||
		!sourceIDsEqual(
			result.Context.Capsules[0].Sources,
			[]string{"CTX_15", "CTX_16", "CTX_17"},
		) {
		t.Fatalf("grouped result=%#v", result)
	}
}

func TestMalformedOptionalSelectionGroupsFailBeforeRelevance(t *testing.T) {
	fixtures := []struct {
		name string
		set  func(*testing.T) CandidateSet
		want string
	}{
		{
			name: "too few members",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, nil, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_1"}}})
			},
			want: "at least two",
		},
		{
			name: "required member",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, []assemblyline.ContextCandidateAuthority{
					candidate(t, "conversation_exchange", "CTX_4", "Required authority."),
				}, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_4", "CTX_1"}}})
			},
			want: "contains required",
		},
		{
			name: "duplicate member",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, nil, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_1", "CTX_1"}}})
			},
			want: "duplicates candidate",
		},
		{
			name: "overlapping groups",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, nil, []OptionalSelectionGroup{
					{CandidateIDs: []string{"CTX_1", "CTX_2"}},
					{CandidateIDs: []string{"CTX_2", "CTX_3"}},
				})
			},
			want: "overlap",
		},
		{
			name: "unknown member",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, nil, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_1", "CTX_99"}}})
			},
			want: "unknown candidate",
		},
		{
			name: "mixed namespaces",
			set: func(t *testing.T) CandidateSet {
				set := groupedCandidateFixture(t, nil, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_1", "CTX_2"}}})
				set.Optional[1] = candidate(t, "durable_memory", "CTX_2", "Optional authority two.")
				return set
			},
			want: "crosses candidate namespaces",
		},
		{
			name: "noncontiguous members",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, nil, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_1", "CTX_3"}}})
			},
			want: "not contiguous",
		},
		{
			name: "reversed members",
			set: func(t *testing.T) CandidateSet {
				return groupedCandidateFixture(t, nil, []OptionalSelectionGroup{{CandidateIDs: []string{"CTX_2", "CTX_1"}}})
			},
			want: "not contiguous",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			relevance := &scriptedRelevanceStation{ids: []string{"CTX_1"}}
			minifier := &scriptedMinificationStation{text: "must not run"}
			_, err := Compile(t.Context(), Request{
				ExactInstruction:   "Use the relevant grouped authority.",
				ModelInstruction:   "Use the relevant grouped authority.",
				KnownArtifactPaths: []string{},
				Retrieval: &RetrievalDirective{
					Availability: SearchAvailable,
				},
			}, &scriptedProvider{set: fixture.set(t)}, Stations{
				Relevance: relevance, Minification: minifier,
			})
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("error=%v, want %q", err, fixture.want)
			}
			if relevance.calls != 0 || minifier.calls != 0 {
				t.Fatalf("invalid group reached relevance/minification %d/%d times", relevance.calls, minifier.calls)
			}
		})
	}
}

func groupedCandidateFixture(
	t *testing.T,
	required []assemblyline.ContextCandidateAuthority,
	groups []OptionalSelectionGroup,
) CandidateSet {
	t.Helper()
	return CandidateSet{
		Required: required,
		Optional: []assemblyline.ContextCandidateAuthority{
			candidate(t, "conversation_exchange", "CTX_1", "Optional authority one."),
			candidate(t, "conversation_exchange", "CTX_2", "Optional authority two."),
			candidate(t, "conversation_exchange", "CTX_3", "Optional authority three."),
		},
		OptionalSelectionGroups: groups,
	}
}

func candidateIDsEqual(
	authorities []assemblyline.ContextCandidateAuthority,
	want []string,
) bool {
	if len(authorities) != len(want) {
		return false
	}
	for index, authority := range authorities {
		if authority.CandidateID != want[index] {
			return false
		}
	}
	return true
}

func sourceIDsEqual(sources []assemblyline.ObjectiveContextSource, want []string) bool {
	if len(sources) != len(want) {
		return false
	}
	for index, source := range sources {
		if source.CandidateID != want[index] {
			return false
		}
	}
	return true
}
