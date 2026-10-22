package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanningActivityRequiresExactPublicFactContract(t *testing.T) {
	for _, activity := range []PlanningActivity{
		{Schema: 1, Kind: "inventory_started"},
		{Schema: 1, Kind: "candidate_started", Subject: strings.Repeat("x", MaxPlanningCandidateSubjectBytes+1)},
		{Schema: 1, Kind: "inventory_completed", Count: 3},
		{Schema: 1, Kind: "candidate_started", Subject: "The user can cancel a pending request."},
		{Schema: 1, Kind: "candidate_resolved", Subject: "The user can cancel a pending request.", Outcome: "retained"},
		{Schema: 1, Kind: "candidate_resolved", Subject: "The user can add and remove items.", Outcome: "partitioned", Count: 2},
		{Schema: 1, Kind: "selected", Count: 2},
	} {
		validator, exists := any(activity).(interface{ Validate() error })
		if !exists {
			t.Fatal("planning observations lack their exact public fact validation")
		}
		if err := validator.Validate(); err != nil {
			t.Errorf("valid code-owned observation rejected: %+v: %v", activity, err)
		}
	}
	for _, activity := range []PlanningActivity{
		{Schema: 2, Kind: "inventory_started"},
		{Schema: 1, Kind: "thinking", Subject: "Invented native thought"},
		{Schema: 1, Kind: "candidate_started"},
		{Schema: 1, Kind: "candidate_started", Subject: " leading space"},
		{Schema: 1, Kind: "candidate_started", Subject: "two\nlines"},
		{Schema: 1, Kind: "candidate_started", Subject: "\xff"},
		{Schema: 1, Kind: "candidate_resolved", Subject: "An outcome.", Outcome: "native-approved"},
		{Schema: 1, Kind: "candidate_resolved", Subject: "An outcome.", Outcome: "retained", Count: 2},
		{Schema: 1, Kind: "inventory_completed", Count: -1},
		{Schema: 1, Kind: "inventory_started", Subject: "Raw prompt"},
		{Schema: 1, Kind: "selected", Outcome: "retained"},
	} {
		validator, exists := any(activity).(interface{ Validate() error })
		if !exists || validator.Validate() == nil {
			t.Errorf("invalid public fact accepted: %+v", activity)
		}
	}
}

func TestPlanningSemanticPublicationAcceptsLargeFrames(t *testing.T) {
	activity := PlanningActivity{Schema: 1, Kind: "semantic_started", WorkKind: "application_requirement_candidate_authorization", Station: "application_requirement_candidate_authorization", Model: "fixture-model", Question: "Which offered value is required?", Why: "Natural language does not determine a literal selection mechanically.", Consumer: "Decode the exact selected value."}
	for i := 0; i < 30; i++ {
		activity.Options = append(activity.Options, strings.Repeat("x", 2048))
	}
	encoded, err := json.Marshal(activity)
	if err != nil || len(encoded) < 48*1024 {
		t.Fatal("oversized publication fixture is invalid")
	}
	if err := activity.Validate(); err != nil {
		t.Fatal("large planning publication was rejected", err)
	}
}
