package webresearch

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWebAcquisitionSourceHasNoSemanticQueryStation(t *testing.T) {
	t.Parallel()
	production := []string{
		"acquisition.go", "evidence_gathering.go", "evidence_gatherer.go", "machine.go", "stations.go",
		"../worker/objective_web_workflow.go", "../worker/objective_roleplay_research.go",
		"../assemblyline/portable_job_registry.go", "../queue/station_gap_mapping.go",
	}
	forbidden := []string{
		"SearchTermsStation", "resolveSearchTerms", "WebSearchTerms", "WorkWebSearchTerm", "WebSearchTermLeaf",
	}
	for _, name := range production {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read production source %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("production source %s retains semantic query authority %q", name, token)
			}
		}
	}
	for _, retired := range []string{
		"../assemblyline/web_search_term_leaves.go",
		"../assemblyline/web_search_terms.go",
	} {
		if _, err := os.Stat(retired); !os.IsNotExist(err) {
			t.Fatalf("retired web query station source %s still exists or cannot be checked: %v", retired, err)
		}
	}
	raw, err := os.ReadFile("attempts.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "websearch.QueryRequest{") != 1 ||
		!strings.Contains(string(raw), "websearch.QueryRequest{Query: config.query}") {
		t.Fatal("web acquisition query construction is not the single code-owned attempt input")
	}
}

func TestStationContractsContainOnlyTheirTypedSemanticLeaf(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		fields []string
	}{
		{name: "relevance call", value: RelevanceCall{}, fields: []string{"Question", "Context", "Candidates", "MaxSelections"}},
		{name: "relevance decision", value: RelevanceDecision{}, fields: []string{"Outcome", "CandidateIDs", "SemanticCalls"}},
		{name: "synthesis call", value: GroundedSynthesisCall{}, fields: []string{"Question", "Context", "Evidence", "MaxParagraphs", "MaxParagraphBytes"}},
		{name: "synthesis decision", value: GroundedSynthesisDecision{}, fields: []string{"Paragraphs", "SemanticCalls"}},
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
		ID: "objective_valid", Question: "What is established?", InitialQuery: "established evidence", Status: ObjectivePending,
	}
	config := Config{
		MaxFetchCandidates: 4,
		MaxProjectionBytes: 1_000, MaxRelevantCandidates: 2, CandidateSummaryBytes: 240,
		MaxSynthesisParagraphs: 4, MaxSynthesisParagraphBytes: 1_000,
	}
	acquisition := &scriptedAcquisition{}
	relevance := &recordingRelevanceStation{}
	synthesis := &recordingSynthesisStation{}
	invalidConfig := config
	invalidConfig.MaxProjectionBytes = 8_193
	if _, err := New(objective, invalidConfig, acquisition, relevance, synthesis); err == nil {
		t.Fatal("projection larger than the portable boundary was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxSynthesisParagraphs = 5
	if _, err := New(objective, invalidConfig, acquisition, relevance, synthesis); err == nil {
		t.Fatal("unbounded synthesis paragraph count was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxSynthesisParagraphBytes = 2_049
	if _, err := New(objective, invalidConfig, acquisition, relevance, synthesis); err == nil {
		t.Fatal("unbounded synthesis paragraph bytes were accepted")
	}
	if _, err := New(objective, config, nil, relevance, synthesis); err == nil {
		t.Fatal("nil deterministic acquisition was accepted")
	}
	if _, err := New(objective, config, acquisition, relevance, nil); err == nil {
		t.Fatal("nil grounded synthesis station was accepted")
	}
	invalidConfig = config
	invalidConfig.MaxFetchCandidates = acquisition.Limits().MaxDocuments + 1
	invalidConfig.MaxRelevantCandidates = acquisition.Limits().MaxDocuments
	if _, err := New(objective, invalidConfig, acquisition, relevance, synthesis); err == nil {
		t.Fatal("workflow fetch bound larger than deterministic acquisition authority was accepted")
	}
}
