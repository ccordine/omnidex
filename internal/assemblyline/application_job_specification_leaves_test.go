package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationJobSpecificationLeavesExposeOneRawSemanticResult(t *testing.T) {
	t.Parallel()
	authority := applicationJobLeafTestAuthority()
	objectivePrompt, err := BuildApplicationJobObjectivePrompt(authority)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(objectivePrompt, "Return only that objective") ||
		strings.Contains(objectivePrompt, "required_behaviors\"") ||
		strings.Contains(objectivePrompt, "acceptance_criteria\"") {
		t.Fatalf("objective prompt crosses its one-leaf boundary:\n%s", objectivePrompt)
	}

	behaviorInput := ApplicationJobBehaviorLeafInput{
		Authority:         authority,
		Objective:         "Implement a browser counter whose displayed value can be increased.",
		AcceptedBehaviors: []string{},
	}
	behaviorPrompt, err := BuildApplicationBehaviorPrompt(behaviorInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(behaviorPrompt, "Return one concrete action-and-result behavior") {
		t.Fatalf("behavior prompt does not request exactly one leaf:\n%s", behaviorPrompt)
	}

	criterionInput := ApplicationJobCriterionLeafInput{
		Authority: authority,
		Objective: behaviorInput.Objective,
		RequiredBehaviors: []string{
			"Activating the increment control increases the displayed count.",
		},
		AcceptedCriteria: []string{},
	}
	criterionPrompt, err := BuildApplicationCriterionPrompt(criterionInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(criterionPrompt, "Return one specific observable acceptance condition") {
		t.Fatalf("criterion prompt does not request exactly one leaf:\n%s", criterionPrompt)
	}
}

func TestApplicationJobSpecificationLeafDecodersRejectStructuredResponses(t *testing.T) {
	t.Parallel()
	authority := applicationJobLeafTestAuthority()
	behaviorInput := ApplicationJobBehaviorLeafInput{
		Authority:         authority,
		Objective:         "Implement a browser counter whose displayed value can be increased.",
		AcceptedBehaviors: []string{},
	}
	criterionInput := ApplicationJobCriterionLeafInput{
		Authority: authority,
		Objective: behaviorInput.Objective,
		RequiredBehaviors: []string{
			"Activating the increment control increases the displayed count.",
		},
		AcceptedCriteria: []string{},
	}
	tests := []struct {
		name   string
		decode func(string) error
	}{
		{
			name: "objective JSON object",
			decode: func(raw string) error {
				_, err := DecodeApplicationJobObjectiveLeaf(authority, raw)
				return err
			},
		},
		{
			name: "behavior JSON string",
			decode: func(raw string) error {
				_, err := DecodeApplicationBehaviorLeaf(behaviorInput, raw)
				return err
			},
		},
		{
			name: "criterion JSON array",
			decode: func(raw string) error {
				_, err := DecodeApplicationCriterionLeaf(criterionInput, raw)
				return err
			},
		},
	}
	candidates := []string{
		`{"objective":"Implement the counter."}`,
		`"Activating the control increases the count."`,
		`["The increased count is visible."]`,
	}
	for index, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.decode(candidates[index]); err == nil {
				t.Fatalf("structured candidate %q was accepted", candidates[index])
			}
		})
	}
}

func TestApplicationJobSpecificationCoverageAndDuplicateBoundaries(t *testing.T) {
	t.Parallel()
	authority := applicationJobLeafTestAuthority()
	behaviorInput := ApplicationJobBehaviorLeafInput{
		Authority: authority,
		Objective: "Implement a browser counter whose displayed value can be increased.",
		AcceptedBehaviors: []string{
			"Activating the increment control increases the displayed count.",
		},
	}
	if value, err := DecodeApplicationBehaviorCoverageLeaf(
		behaviorInput, ApplicationNoUncoveredBehavior,
	); err != nil || value != ApplicationNoUncoveredBehavior {
		t.Fatalf("behavior coverage=%q err=%v", value, err)
	}
	if _, err := DecodeApplicationBehaviorLeaf(
		behaviorInput, behaviorInput.AcceptedBehaviors[0],
	); err == nil {
		t.Fatal("duplicate behavior was accepted")
	}

	criterionInput := ApplicationJobCriterionLeafInput{
		Authority:         authority,
		Objective:         behaviorInput.Objective,
		RequiredBehaviors: append([]string(nil), behaviorInput.AcceptedBehaviors...),
		AcceptedCriteria: []string{
			"The displayed count is one greater after the increment control is activated.",
		},
	}
	if value, err := DecodeApplicationCriterionCoverageLeaf(
		criterionInput, ApplicationCriterionRemains,
	); err != nil || value != ApplicationCriterionRemains {
		t.Fatalf("criterion coverage=%q err=%v", value, err)
	}
	if _, err := DecodeApplicationCriterionLeaf(
		criterionInput, criterionInput.AcceptedCriteria[0],
	); err == nil {
		t.Fatal("duplicate criterion was accepted")
	}
	if _, err := DecodeApplicationCriterionCoverageLeaf(criterionInput, "accept"); err == nil {
		t.Fatal("unregistered control label was accepted as a semantic relation")
	}
}

func applicationJobLeafTestAuthority() ApplicationJobSpecificationInput {
	requirement := Requirement{
		ID:          "requirement_001",
		SourceQuote: "Display a count and increase it when an increment control is activated.",
	}
	return ApplicationJobSpecificationInput{
		Surface:              ApplicationSurfaceBrowser,
		ProductQuote:         "A browser counter",
		AcceptedRequirements: []Requirement{requirement},
		FocusedRequirement:   requirement,
	}
}
