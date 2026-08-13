package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func webSearchTermsFixture() WebSearchTermsInput {
	return WebSearchTermsInput{
		ExactQuestion:    "  Which release is current?\n",
		AttemptedQueries: []string{"current release"},
		MaxTerms:         3,
		MaxTermBytes:     80,
	}
}

func TestWebSearchTermsPortableContractPreservesExactQuestion(t *testing.T) {
	input := webSearchTermsFixture()
	job, err := NewWebSearchTermsJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkWebSearchTerms {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"exact_question":"  Which release is current?\n"`) {
		t.Fatalf("prompt did not preserve exact question: %s", prompt)
	}
	if schema["additionalProperties"] != false {
		t.Fatal("response schema permits extra authority fields")
	}
	raw := fmt.Sprintf(`{"schema":%q,"terms":["release notes","stable version"]}`, WebSearchTermsSchemaV1)
	decision, err := DecodeWebSearchTermsDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Terms) != 2 || decision.Terms[0] != "release notes" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestWebSearchTermsAllowsNoAttemptedQueryForOversizedExactQuestion(t *testing.T) {
	input := WebSearchTermsInput{
		ExactQuestion: strings.Repeat("question ", 300), AttemptedQueries: []string{},
		MaxTerms: 2, MaxTermBytes: 80,
	}
	job, err := NewWebSearchTermsJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"attempted_queries":[]`) || !strings.Contains(prompt, input.ExactQuestion) {
		t.Fatal("empty attempted-query gap lost the exact question")
	}
}

func TestWebSearchTermsRejectsAmbiguousOrUnboundedResults(t *testing.T) {
	input := webSearchTermsFixture()
	validSchema := fmt.Sprintf(`"schema":%q`, WebSearchTermsSchemaV1)
	tests := map[string]string{
		"duplicate field": `{` + validSchema + `,"terms":["release"],"terms":["stable"]}`,
		"case alias":      `{` + validSchema + `,"Terms":["release"]}`,
		"unknown field":   `{` + validSchema + `,"terms":["release"],"action":"search"}`,
		"trailing value":  `{` + validSchema + `,"terms":["release"]} {}`,
		"markdown fence":  "```json\n{" + validSchema + `,"terms":["release"]}` + "\n```",
		"duplicate term":  `{` + validSchema + `,"terms":["Release","release"]}`,
		"attempted query": `{` + validSchema + `,"terms":["CURRENT RELEASE"]}`,
		"too many":        `{` + validSchema + `,"terms":["one","two","three","four"]}`,
		"oversized term":  `{` + validSchema + `,"terms":["` + strings.Repeat("x", input.MaxTermBytes+1) + `"]}`,
		"oversized raw":   strings.Repeat("x", maxPortableCandidateBytes+1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeWebSearchTermsDecision(input, raw); err == nil {
				t.Fatal("invalid search-term result was accepted")
			}
		})
	}
}

func TestWebSearchTermsRejectsOversizedInputBeforePortableEncoding(t *testing.T) {
	input := webSearchTermsFixture()
	input.ExactQuestion = strings.Repeat("x", maxWebQuestionBytes+1)
	if _, err := NewWebSearchTermsJob(input); err == nil {
		t.Fatal("oversized question was accepted")
	}
	input = webSearchTermsFixture()
	input.AttemptedQueries = []string{"one", "two", "three", "four", "five"}
	if _, err := NewWebSearchTermsJob(input); err == nil {
		t.Fatal("oversized attempted-query set was accepted")
	}
}
