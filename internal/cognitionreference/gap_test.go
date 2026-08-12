package cognitionreference

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeGapSelector struct {
	selected     CandidateID
	err          error
	calls        int
	received     SemanticGap
	beforeReturn func()
}

func (selector *fakeGapSelector) Select(_ context.Context, gap SemanticGap) (CandidateID, error) {
	selector.calls++
	selector.received = gap.Clone()
	gap.Evidence[0].Content = "mutated by selector"
	gap.Candidates[0].EvidenceIDs[0] = "mutated"
	if selector.beforeReturn != nil {
		selector.beforeReturn()
	}
	return selector.selected, selector.err
}

func TestSelectCandidateExposesOneExactClonedSemanticGap(t *testing.T) {
	t.Parallel()
	gap := validSemanticGap()
	want := gap.Clone()
	selector := &fakeGapSelector{selected: "C17"}

	selected, err := SelectCandidate(t.Context(), selector, gap)
	if err != nil {
		t.Fatal(err)
	}
	if selected != "C17" || selector.calls != 1 {
		t.Fatalf("selection=%q calls=%d, want C17 and one call", selected, selector.calls)
	}
	if !reflect.DeepEqual(selector.received, want) || !reflect.DeepEqual(gap, want) {
		t.Fatalf("selector did not receive an isolated exact clone: received=%#v gap=%#v", selector.received, gap)
	}
}

func TestSelectCandidateRejectsEveryNonMemberWithoutFallback(t *testing.T) {
	t.Parallel()
	for _, selected := range []CandidateID{"", "C99", " C17", "C17 "} {
		selected := selected
		t.Run(string(selected), func(t *testing.T) {
			t.Parallel()
			selector := &fakeGapSelector{selected: selected}
			if _, err := SelectCandidate(t.Context(), selector, validSemanticGap()); !errors.Is(err, ErrInvalidSelection) {
				t.Fatalf("selection %q error=%v, want ErrInvalidSelection", selected, err)
			}
			if selector.calls != 1 {
				t.Fatalf("selection %q calls=%d, want exactly one", selected, selector.calls)
			}
		})
	}
}

func TestSemanticGapRejectsNonCanonicalOrUngroundedInputs(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*SemanticGap){
		"unknown kind":  func(gap *SemanticGap) { gap.Kind = "planner" },
		"one candidate": func(gap *SemanticGap) { gap.Candidates = gap.Candidates[:1] },
		"unsorted candidates": func(gap *SemanticGap) {
			gap.Candidates[0], gap.Candidates[1] = gap.Candidates[1], gap.Candidates[0]
		},
		"unknown evidence": func(gap *SemanticGap) { gap.Candidates[0].EvidenceIDs[0] = "E99" },
		"unreferenced evidence": func(gap *SemanticGap) {
			gap.Evidence = append(gap.Evidence, SemanticEvidence{ID: "E30", Content: "Irrelevant."})
		},
		"inexact question": func(gap *SemanticGap) { gap.Question = " padded " },
		"oversized summary": func(gap *SemanticGap) {
			gap.Candidates[0].Summary = strings.Repeat("x", maxCandidateSummaryBytes+1)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gap := validSemanticGap()
			mutate(&gap)
			if err := gap.Validate(); !errors.Is(err, ErrInvalidSemanticGap) {
				t.Fatalf("Validate() error=%v, want ErrInvalidSemanticGap", err)
			}
		})
	}
}

func TestSelectCandidatePropagatesSelectorFailureWithoutGuessing(t *testing.T) {
	t.Parallel()
	want := errors.New("provider failed")
	selector := &fakeGapSelector{selected: "C17", err: want}
	if _, err := SelectCandidate(t.Context(), selector, validSemanticGap()); !errors.Is(err, want) {
		t.Fatalf("SelectCandidate() error=%v, want provider failure", err)
	}
	if selector.calls != 1 {
		t.Fatalf("selector calls=%d, want one", selector.calls)
	}
}

func TestSelectCandidateRejectsUnavailableContextWithoutCallingSelector(t *testing.T) {
	t.Parallel()
	selector := &fakeGapSelector{selected: "C17"}
	if _, err := SelectCandidate(nil, selector, validSemanticGap()); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("nil context error=%v, want ErrInvalidSelection", err)
	}
	if selector.calls != 0 {
		t.Fatalf("nil context selector calls=%d, want zero", selector.calls)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := SelectCandidate(ctx, selector, validSemanticGap()); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled context error=%v, want context.Canceled", err)
	}
	if selector.calls != 0 {
		t.Fatalf("pre-canceled context selector calls=%d, want zero", selector.calls)
	}
}

func TestSelectCandidateDiscardsValidOutputWhenContextCancelsDuringCall(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	selector := &fakeGapSelector{selected: "C17", beforeReturn: cancel}
	selected, err := SelectCandidate(ctx, selector, validSemanticGap())
	if !errors.Is(err, context.Canceled) || selected != "" {
		t.Fatalf("selection=%q error=%v, want empty and context.Canceled", selected, err)
	}
	if selector.calls != 1 {
		t.Fatalf("selector calls=%d, want exactly one", selector.calls)
	}
}

func TestSelectCandidateValidatesAgainstFrozenEntrySnapshot(t *testing.T) {
	t.Parallel()
	gap := validSemanticGap()
	selector := &fakeGapSelector{selected: "C18"}
	selector.beforeReturn = func() { gap.Candidates[0].ID = "C18" }
	selected, err := SelectCandidate(t.Context(), selector, gap)
	if !errors.Is(err, ErrInvalidSelection) || selected != "" {
		t.Fatalf("selection=%q error=%v, want empty and ErrInvalidSelection", selected, err)
	}
	if selector.received.Candidates[0].ID != "C17" {
		t.Fatalf("selector-visible candidate mutated to %q", selector.received.Candidates[0].ID)
	}
}

func validSemanticGap() SemanticGap {
	return SemanticGap{
		ID: "gap.route-meaning", Kind: GapCandidateSelection,
		ObjectiveID: "objective.reach-destination",
		Question:    "Which equally supported route interpretation should this objective retain?",
		Evidence: []SemanticEvidence{
			{ID: "E10", Content: "The authoritative clue permits either a sheltered or exposed route."},
			{ID: "E20", Content: "Both routes are legal, equal-cost, and equally supported; no registered fact breaks the tie."},
		},
		Candidates: []SemanticCandidate{
			{ID: "C17", Summary: "Retain the sheltered interpretation.", EvidenceIDs: []EvidenceID{"E10", "E20"}},
			{ID: "C23", Summary: "Retain the exposed interpretation.", EvidenceIDs: []EvidenceID{"E10", "E20"}},
		},
	}
}
