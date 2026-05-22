package omni

import "testing"

func TestStructuredRolePromptBudgetChars(t *testing.T) {
	cases := map[string]int{
		structuredBudgetRolePlanner:    defaultStructuredPlannerPromptBudgetChars,
		structuredBudgetRoleShell:      defaultStructuredShellPromptBudgetChars,
		structuredBudgetRoleEvaluator:  defaultStructuredEvaluatorPromptBudgetChars,
		structuredBudgetRoleCompletion: defaultStructuredCompletionPromptBudgetChars,
		"unknown":                      defaultStructuredPlannerPromptBudgetChars,
	}
	for role, want := range cases {
		if got := structuredRolePromptBudgetChars(role); got != want {
			t.Fatalf("budget(%q)=%d want %d", role, got, want)
		}
	}
}
