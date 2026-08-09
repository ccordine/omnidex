package llm

import (
	"strings"
	"testing"
)

func TestAdvisoryResponsePreservesThinkingAndContentAsExactEvidence(t *testing.T) {
	response := AdvisoryResponse{
		Thinking: "\nreason through the failure\n",
		Content:  "\nconcise critique\n",
	}
	if err := response.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := response.EvidenceJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{`"thinking":"\nreason through the failure\n"`, `"content":"\nconcise critique\n"`} {
		if !strings.Contains(raw, exact) {
			t.Fatalf("evidence %s omitted %s", raw, exact)
		}
	}
	decoded, err := DecodeAdvisoryEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != response {
		t.Fatalf("decoded=%#v want %#v", decoded, response)
	}
}

func TestAdvisoryResponseRejectsEmptyOrUnknownEvidence(t *testing.T) {
	if err := (AdvisoryResponse{}).Validate(); err == nil {
		t.Fatal("empty advisory response was accepted")
	}
	if _, err := DecodeAdvisoryEvidence(`{"thinking":"useful","extra":true}`); err == nil {
		t.Fatal("advisory evidence accepted an unknown field")
	}
}
