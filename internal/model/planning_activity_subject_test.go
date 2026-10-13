package model

import (
	"strings"
	"testing"
)

func TestPlanningActivitySemanticSubjectPreservesSourceGrammar(t *testing.T) {
	for _, subject := range []string{" A valid fictional fact. ", "The left behavior.\nThe next line.", strings.Repeat("x", 2000)} {
		activity := PlanningActivity{Schema: 1, Kind: "semantic_started", WorkKind: "capability_relation",
			Station: "capability_relation", Model: "fixture-model", Subject: subject, Against: subject,
			Question: "What dependency exists?", Why: "Natural language requires interpretation.",
			Consumer: "Consume the decoded relation.", Options: []string{"A. Independent", "B. Dependent"},
		}
		if err := activity.Validate(); err != nil {
			t.Fatalf("valid source grammar became a display gate: %v", err)
		}
	}
	for _, subject := range []string{strings.Repeat("x", 2001), "invalid\x00subject", string([]byte{0xff})} {
		activity := PlanningActivity{Schema: 1, Kind: "semantic_started", WorkKind: "capability_relation",
			Station: "capability_relation", Model: "fixture-model", Subject: subject,
			Question: "What dependency exists?", Why: "Natural language requires interpretation.",
			Consumer: "Consume the decoded relation.", Options: []string{"A. Independent", "B. Dependent"},
		}
		if activity.Validate() == nil {
			t.Fatalf("invalid source text escaped the public display bound: %q", subject)
		}
	}
}
