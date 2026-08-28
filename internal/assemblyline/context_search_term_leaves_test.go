package assemblyline

import (
	"strings"
	"testing"
)

func TestContextSearchTermLeavesBuildOneRawConceptAtATime(t *testing.T) {
	input := ContextSearchTermLeafInput{
		ExactInstruction: "Do that again.", AcceptedTerms: []string{},
	}
	coverageJob, err := NewContextSearchTermCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(coverageJob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, ContextTermRemains) ||
		strings.Contains(prompt, `"terms"`) {
		t.Fatalf("prompt=%q", prompt)
	}
	coverage, err := DecodeContextSearchTermCoverageLeaf(input, ContextTermRemains)
	if err != nil || coverage != ContextTermRemains {
		t.Fatalf("coverage=%q error=%v", coverage, err)
	}
	termJob, err := NewContextSearchTermJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderPortableJob(termJob); err != nil {
		t.Fatalf("render term: %v", err)
	}
	term, err := DecodeContextSearchTermLeaf(input, "previous action")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := AssembleContextSearchTermsDecision(
		ContextSearchTermsInput{ExactInstruction: input.ExactInstruction},
		[]string{term},
	)
	if err != nil || len(decision.Terms) != 1 || decision.Terms[0] != term {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestContextSearchTermLeavesRejectStructuredDuplicateAndUnboundedValues(t *testing.T) {
	input := ContextSearchTermLeafInput{
		ExactInstruction: "Do that again.", AcceptedTerms: []string{"previous action"},
	}
	for _, raw := range []string{
		`{"term":"earlier action"}`,
		`"earlier action"`,
		"previous action",
		strings.Repeat("x", MaxContextSearchTermBytes+1),
	} {
		if _, err := DecodeContextSearchTermLeaf(input, raw); err == nil {
			t.Fatalf("accepted invalid term %q", raw)
		}
	}
	if _, err := DecodeContextSearchTermCoverageLeaf(input, `{"coverage":"NO_UNCOVERED_CONTEXT_TERM"}`); err == nil {
		t.Fatal("accepted structured coverage leaf")
	}
}
