package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCanonicalReplanTailUsesOnlyRegisteredBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		seeds    []stepSeed
		boundary string
		sort     int
		tail     []string
	}{
		{
			name:     "direct coding",
			seeds:    []stepSeed{{action: "v3_coding", sortIndex: 5}},
			boundary: "v3_coding",
			sort:     5,
			tail:     []string{"v3_coding"},
		},
		{
			name:     "conversation objective",
			seeds:    conversationObjectiveSteps(),
			boundary: "objective_resolve",
			sort:     5,
			tail:     []string{"objective_resolve"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary, err := canonicalReplanTail(test.seeds)
			if err != nil {
				t.Fatal(err)
			}
			if boundary.action != test.boundary || boundary.sortIndex != test.sort {
				t.Fatalf("boundary=%+v", boundary)
			}
			if got := seedActions(boundary.seeds); strings.Join(got, ",") != strings.Join(test.tail, ",") {
				t.Fatalf("tail=%v want %v", got, test.tail)
			}
		})
	}
}

func TestCanonicalReplanTailRejectsMissingOrDuplicateBoundary(t *testing.T) {
	invalid := [][]stepSeed{
		{{action: "plan", sortIndex: 40}},
		{{action: "v3_analysis", sortIndex: 80}},
		{{action: "objective_resolve", sortIndex: 1}, {action: "objective_resolve", sortIndex: 2}},
		{{action: "v3_coding", sortIndex: 5}, {action: "v3_coding", sortIndex: 10}},
	}
	for _, seeds := range invalid {
		if _, err := canonicalReplanTail(seeds); err == nil {
			t.Fatalf("invalid canonical seeds accepted: %+v", seeds)
		}
	}
}

func TestValidateCurrentReplanTailAcceptsObjectiveBoundary(t *testing.T) {
	canonical, err := canonicalReplanTail(conversationObjectiveSteps())
	if err != nil {
		t.Fatal(err)
	}
	rows := []replanStepRecord{
		{ID: 11, Action: "objective_resolve", SortIndex: 5, Status: model.StepStatusRunning, Generation: 3},
	}
	ids, err := validateCurrentReplanTail(3, canonical, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 11 {
		t.Fatalf("retiring ids=%v", ids)
	}
}

func TestValidateCurrentReplanTailRejectsCorruption(t *testing.T) {
	canonical, err := canonicalReplanTail([]stepSeed{
		{action: "objective_resolve", sortIndex: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	valid := []replanStepRecord{
		{ID: 1, Action: "objective_resolve", SortIndex: 5, Status: model.StepStatusCompleted, Generation: 2},
	}
	tests := map[string][]replanStepRecord{
		"wrong generation": {
			{ID: 1, Action: "objective_resolve", SortIndex: 5, Status: model.StepStatusCompleted, Generation: 1},
		},
		"wrong boundary sort": {
			{ID: 1, Action: "objective_resolve", SortIndex: 6, Status: model.StepStatusCompleted, Generation: 2},
		},
		"unknown action": {
			{ID: 1, Action: "unregistered", SortIndex: 5, Status: model.StepStatusCompleted, Generation: 2},
		},
		"duplicate canonical seed": {valid[0], {ID: 2, Action: "objective_resolve", SortIndex: 6, Status: model.StepStatusWaiting, Generation: 2}},
	}
	for name, rows := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := validateCurrentReplanTail(2, canonical, rows); err == nil {
				t.Fatalf("corrupt tail accepted: %+v", rows)
			}
		})
	}
}

func TestValidateReplanFeedbackProducesExactBoundedDigest(t *testing.T) {
	feedback, digest, err := validateReplanFeedback("  Keep the accepted invariant.  \n")
	if err != nil {
		t.Fatal(err)
	}
	if feedback != "Keep the accepted invariant." {
		t.Fatalf("feedback=%q", feedback)
	}
	want := sha256.Sum256([]byte(feedback))
	if digest != hex.EncodeToString(want[:]) {
		t.Fatalf("digest=%q", digest)
	}
	for name, invalid := range map[string]string{
		"empty":        " \n\t ",
		"nul":          "invalid\x00feedback",
		"invalid utf8": string([]byte{0xff}),
		"oversized":    strings.Repeat("x", maxReplanFeedbackBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := validateReplanFeedback(invalid); err == nil {
				t.Fatal("invalid feedback accepted")
			}
		})
	}
}

func seedActions(seeds []stepSeed) []string {
	actions := make([]string, len(seeds))
	for index, seed := range seeds {
		actions[index] = seed.action
	}
	return actions
}
