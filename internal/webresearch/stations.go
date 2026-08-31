package webresearch

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

type RelevanceCandidate struct {
	CandidateID websearch.CandidateID
	Title       string
	Snippet     string
	Excerpt     string
}

type RelevanceCall struct {
	Question      string
	Context       assemblyline.ObjectiveContext
	Candidates    []RelevanceCandidate
	MaxSelections int
}

type RelevanceDecision struct {
	Outcome       RelevanceOutcome
	CandidateIDs  []websearch.CandidateID
	SemanticCalls int
	CallLedger    SemanticCallLedger
}

type RelevanceOutcome string

const (
	RelevanceSelected RelevanceOutcome = "selected"
	RelevanceNone     RelevanceOutcome = "none"
)

type RelevanceStation interface {
	Select(context.Context, RelevanceCall) (RelevanceDecision, error)
}

type ProjectedEvidence struct {
	EvidenceID  EvidenceID
	CandidateID websearch.CandidateID
	Title       string
	Snippet     string
	Content     string
	Truncated   bool
}

type GroundedParagraph struct {
	Text        string
	EvidenceIDs []EvidenceID
}

// Acquisition is code-operated web mechanics. It is deliberately not exposed
// to any station and has no model-visible operation or action representation.
type Acquisition interface {
	Limits() websearch.AcquisitionLimits
	Discover(context.Context, websearch.QueryRequest) (websearch.CandidateReport, error)
	Fetch(context.Context, websearch.FetchRequest) (websearch.DocumentReport, error)
}
