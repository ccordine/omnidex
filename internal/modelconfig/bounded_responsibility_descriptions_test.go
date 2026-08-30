package modelconfig

import (
	"strings"
	"testing"
)

func TestBoundedSieveModelDescriptionsAreExact(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"context_relevance_model":          "Judges one code-known context candidate against one exact instruction",
		"grounded_answer_model":            "Returns one bounded untrusted paragraph inventory, one evidence-support relation, or one paragraph-authorization relation per repository-grounding call",
		"web_relevance_model":              "Judges one code-known web evidence candidate against one exact question",
		"web_grounded_synthesis_model":     "Returns one bounded untrusted paragraph inventory, one evidence-support relation, or one paragraph-authorization relation per web-grounding call",
		"coding_requirements_model":        "Returns one bounded product or stack value, one untrusted context-question or requirement inventory, or one candidate-bound sieve result per coding-intent call",
		"coding_capability_relation_model": "Resolves one pairwise direct-dependency relation or one candidate-bound runtime-necessity relation per call",
	}
	got := make(map[string]string, len(want))
	for _, field := range Fields {
		if _, required := want[field.Key]; required {
			got[field.Key] = field.Description
		}
	}
	for key, description := range want {
		if got[key] != description {
			t.Errorf("%s description=%q want %q", key, got[key], description)
		}
		lower := strings.ToLower(got[key])
		for _, forbidden := range []string{
			"select", "synthesi", "global", "exhaust", "complet",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s description retains set/global authority %q: %q", key, forbidden, got[key])
			}
		}
	}
}
