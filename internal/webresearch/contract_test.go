package webresearch

import (
	"reflect"
	"testing"
)

func TestStationContractsContainOnlyTheirTypedSemanticLeaf(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		fields []string
	}{
		{name: "search terms call", value: SearchTermsCall{}, fields: []string{"Question", "Context", "AttemptedQueries", "MaxTerms", "MaxTermBytes"}},
		{name: "search terms decision", value: SearchTermsDecision{}, fields: []string{"Terms", "SemanticCalls"}},
		{name: "relevance call", value: RelevanceCall{}, fields: []string{"Question", "Context", "Candidates", "MaxSelections"}},
		{name: "relevance decision", value: RelevanceDecision{}, fields: []string{"Outcome", "CandidateIDs", "SemanticCalls"}},
		{name: "synthesis call", value: GroundedSynthesisCall{}, fields: []string{"Question", "Context", "Evidence", "MaxParagraphs", "MaxParagraphBytes"}},
		{name: "synthesis decision", value: GroundedSynthesisDecision{}, fields: []string{"Paragraphs", "SemanticCalls"}},
		{name: "claim evidence review call", value: ClaimEvidenceReviewCall{}, fields: []string{"Question", "Context", "ParagraphID", "ParagraphText", "Evidence"}},
		{name: "claim evidence review decision", value: ClaimEvidenceReviewDecision{}, fields: []string{"Outcome", "ParagraphID", "EvidenceIDs", "IssueKind", "Detail", "SemanticCalls"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typeOf := reflect.TypeOf(test.value)
			if typeOf.NumField() != len(test.fields) {
				t.Fatalf("fields=%d want %d", typeOf.NumField(), len(test.fields))
			}
			for index, field := range test.fields {
				if typeOf.Field(index).Name != field {
					t.Fatalf("field %d=%s want %s", index, typeOf.Field(index).Name, field)
				}
			}
		})
	}
}

func TestNewRejectsInvalidMachineAuthority(t *testing.T) {
	objective := Objective{
		ID: "objective_valid", Question: "What is established?", InitialQuery: "established evidence",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}
	config := Config{
		MaxSearchTerms: 3, MaxSearchTermBytes: 120, MaxFetchCandidates: 4,
		MaxProjectionBytes: 1_000, MaxRelevantCandidates: 2, CandidateSummaryBytes: 240,
		MaxSynthesisParagraphs: 4, MaxSynthesisParagraphBytes: 1_000,
	}
	acquisition := &scriptedAcquisition{}
	terms := &recordingTermsStation{}
	relevance := &recordingRelevanceStation{}
	synthesis := &recordingSynthesisStation{}
	correction := &recordingSynthesisCorrectionStation{}
	review := &recordingClaimEvidenceReviewStation{}

	invalidObjective := objective
	invalidObjective.Acceptance = []AcceptancePredicate{AcceptanceGroundedSynthesis}
	if _, err := New(invalidObjective, config, acquisition, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("missing exact citation acceptance was accepted")
	}
	invalidConfig := config
	invalidConfig.MaxSearchTerms = 4
	if _, err := New(objective, invalidConfig, acquisition, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("unbounded search term count was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxProjectionBytes = 8_193
	if _, err := New(objective, invalidConfig, acquisition, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("projection larger than the portable boundary was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxSynthesisParagraphs = 5
	if _, err := New(objective, invalidConfig, acquisition, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("unbounded synthesis paragraph count was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxSynthesisParagraphBytes = 2_049
	if _, err := New(objective, invalidConfig, acquisition, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("unbounded synthesis paragraph bytes were accepted")
	}
	if _, err := New(objective, config, nil, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("nil deterministic acquisition was accepted")
	}
	if _, err := New(objective, config, acquisition, terms, relevance, synthesis, correction, nil); err == nil {
		t.Fatal("nil independent claim-evidence review was accepted")
	}
	if _, err := New(objective, config, acquisition, terms, relevance, synthesis, nil, review); err == nil {
		t.Fatal("nil bounded synthesis correction station was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxFetchCandidates = acquisition.Limits().MaxDocuments + 1
	invalidConfig.MaxRelevantCandidates = acquisition.Limits().MaxDocuments
	if _, err := New(objective, invalidConfig, acquisition, terms, relevance, synthesis, correction, review); err == nil {
		t.Fatal("workflow fetch bound larger than deterministic acquisition authority was accepted")
	}
}
