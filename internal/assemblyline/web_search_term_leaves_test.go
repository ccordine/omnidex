package assemblyline

import (
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

func TestWebSearchTermsUseRawCoverageAndOneTermLeaves(t *testing.T) {
	t.Parallel()
	base := webSearchTermsFixture()
	input := WebSearchTermLeafInput{
		ExactQuestion:    base.ExactQuestion,
		Context:          base.Context,
		AttemptedQueries: append([]string{}, base.AttemptedQueries...),
		AcceptedTerms:    []string{},
		MaxTerms:         base.MaxTerms,
		MaxTermBytes:     base.MaxTermBytes,
	}

	coverageJob, err := NewWebSearchTermCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	termJob, err := NewWebSearchTermJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if coverageJob.Kind != WorkWebSearchTermCoverage || termJob.Kind != WorkWebSearchTerm {
		t.Fatalf("leaf kinds=(%q,%q)", coverageJob.Kind, termJob.Kind)
	}
	for _, job := range []PortableJob{coverageJob, termJob} {
		prompt, err := RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(prompt, `"exact_question":"  Which release is current?\n"`) ||
			!strings.Contains(prompt, base.AttemptedQueries[0]) {
			t.Fatalf("web search leaf lost exact bounded authority: %s", prompt)
		}
		for _, forbidden := range []string{`"schema"`, `"terms"`, "terms array"} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("web search leaf prompt exposes aggregate response field %q: %s", forbidden, prompt)
			}
		}
	}

	coverage, err := DecodeWebSearchTermCoverageLeaf(input, string(WebQueryTermRemains))
	if err != nil || coverage.Coverage != WebQueryTermRemains {
		t.Fatalf("coverage=%+v error=%v", coverage, err)
	}
	term, err := DecodeWebSearchTermLeaf(input, "stable version release notes")
	if err != nil || term.Term != "stable version release notes" {
		t.Fatalf("term=%+v error=%v", term, err)
	}
	assembled, err := AssembleWebSearchTermsDecision(base, []string{term.Term})
	if err != nil {
		t.Fatal(err)
	}
	if assembled.Schema != WebSearchTermsSchemaV1 || len(assembled.Terms) != 1 ||
		assembled.Terms[0] != term.Term {
		t.Fatalf("code-owned decision=%+v", assembled)
	}
}

func TestWebSearchTermLeavesRejectStructuredDuplicateAndExhaustedResults(t *testing.T) {
	t.Parallel()
	base := webSearchTermsFixture()
	input := WebSearchTermLeafInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		AttemptedQueries: append([]string{}, base.AttemptedQueries...),
		AcceptedTerms:    []string{"stable version"},
		MaxTerms:         base.MaxTerms, MaxTermBytes: base.MaxTermBytes,
	}
	for _, raw := range []string{
		`{"term":"release notes"}`,
		`["release notes"]`,
		`"release notes"`,
		" release notes ",
		base.AttemptedQueries[0],
		"STABLE VERSION",
	} {
		if _, err := DecodeWebSearchTermLeaf(input, raw); err == nil {
			t.Fatalf("invalid raw search term leaf accepted: %q", raw)
		}
	}
	if _, err := DecodeWebSearchTermCoverageLeaf(
		input, `{"coverage":"NO_UNCOVERED_QUERY_TERM"}`,
	); err == nil {
		t.Fatal("structured search coverage was accepted")
	}
	exhausted := input
	exhausted.MaxTerms = 2
	exhausted.AcceptedTerms = []string{"stable version", "release notes"}
	if _, err := NewWebSearchTermJob(exhausted); err == nil {
		t.Fatal("term job was created after its fixed-point bound was exhausted")
	}
}
