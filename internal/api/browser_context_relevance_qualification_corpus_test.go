package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const browserContextQualificationCorpusSchemaV1 = "omnidex.context-relevance-qualification-corpus.v1"

//go:embed testdata/browser_context_relevance_qualification.v1.json
var browserContextQualificationCorpusJSON []byte

type browserContextQualificationCorpus struct {
	Schema  string                            `json:"schema"`
	Version string                            `json:"version"`
	Cases   []browserContextQualificationCase `json:"cases"`
}

type browserContextQualificationCase struct {
	Name                 string                                 `json:"name"`
	ExactInstruction     string                                 `json:"exact_instruction"`
	RetrievalConcepts    []string                               `json:"retrieval_concepts"`
	MaxSelections        int                                    `json:"max_selections"`
	Candidates           []browserContextQualificationCandidate `json:"candidates"`
	ExpectedCandidateIDs []string                               `json:"expected_candidate_ids"`
}

type browserContextQualificationCandidate struct {
	Namespace   string `json:"namespace"`
	CandidateID string `json:"candidate_id"`
	Content     string `json:"content"`
}

func TestBrowserContextRelevanceQualificationCorpusIsValid(t *testing.T) {
	corpus := loadBrowserContextQualificationCorpus(t)
	if len(corpus.Cases) != 10 {
		t.Fatalf("qualification corpus cases=%d want=10", len(corpus.Cases))
	}
	seen := make(map[string]struct{}, len(corpus.Cases))
	for index, testCase := range corpus.Cases {
		if testCase.Name == "" || testCase.Name != strings.TrimSpace(testCase.Name) {
			t.Fatalf("case %d has invalid name %q", index, testCase.Name)
		}
		if _, duplicate := seen[testCase.Name]; duplicate {
			t.Fatalf("duplicate qualification case %q", testCase.Name)
		}
		seen[testCase.Name] = struct{}{}
		input := buildBrowserContextQualificationInput(t, testCase)
		if _, err := assemblyline.NewContextRelevanceSelectionJob(
			assemblyline.ContextRelevanceSelectionInput{
				Authority: input, AcceptedCandidateIDs: []string{},
			},
		); err != nil {
			t.Fatalf("case %q does not satisfy the production station contract: %v", testCase.Name, err)
		}
		expected := assemblyline.ContextRelevanceDecision{
			Schema:                 assemblyline.ContextRelevanceSchemaV1,
			ReferencedCandidateIDs: append([]string{}, testCase.ExpectedCandidateIDs...),
		}
		if err := expected.ValidateFor(input); err != nil {
			t.Fatalf("case %q has invalid expected selection: %v", testCase.Name, err)
		}
	}
}

func loadBrowserContextQualificationCorpus(t *testing.T) browserContextQualificationCorpus {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(browserContextQualificationCorpusJSON))
	decoder.DisallowUnknownFields()
	var corpus browserContextQualificationCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("decode browser context qualification corpus: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("browser context qualification corpus contains trailing JSON: %v", err)
	}
	if corpus.Schema != browserContextQualificationCorpusSchemaV1 {
		t.Fatalf("qualification corpus schema=%q", corpus.Schema)
	}
	if corpus.Version == "" || corpus.Version != strings.TrimSpace(corpus.Version) {
		t.Fatalf("qualification corpus version=%q", corpus.Version)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("qualification corpus has no cases")
	}
	return corpus
}

func buildBrowserContextQualificationInput(
	t *testing.T,
	testCase browserContextQualificationCase,
) assemblyline.ContextRelevanceInput {
	t.Helper()
	candidates := make([]assemblyline.ContextCandidateAuthority, len(testCase.Candidates))
	for index, candidate := range testCase.Candidates {
		authority, err := assemblyline.NewContextCandidateAuthority(
			candidate.Namespace, candidate.CandidateID, candidate.Content,
		)
		if err != nil {
			t.Fatalf("case %q candidate %d: %v", testCase.Name, index, err)
		}
		candidates[index] = authority
	}
	return assemblyline.ContextRelevanceInput{
		ExactInstruction:     testCase.ExactInstruction,
		RetrievalConcepts:    append([]string(nil), testCase.RetrievalConcepts...),
		CandidateAuthorities: candidates,
		MaxSelections:        testCase.MaxSelections,
	}
}

func qualificationCaseLabel(index int, testCase browserContextQualificationCase) string {
	return fmt.Sprintf("%02d/%s", index+1, testCase.Name)
}
