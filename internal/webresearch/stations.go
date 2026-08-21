package webresearch

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

type SearchTermsCall struct {
	Question         string
	Context          assemblyline.ObjectiveContext
	AttemptedQueries []string
	MaxTerms         int
	MaxTermBytes     int
}

type SearchTermsDecision struct {
	Terms []string
}

type SearchTermsStation interface {
	Resolve(context.Context, SearchTermsCall) (SearchTermsDecision, error)
}

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
	Outcome      RelevanceOutcome
	CandidateIDs []websearch.CandidateID
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

type GroundedSynthesisCall struct {
	Question          string
	Context           assemblyline.ObjectiveContext
	Evidence          []ProjectedEvidence
	MaxParagraphs     int
	MaxParagraphBytes int
}

type GroundedParagraph struct {
	Text        string
	EvidenceIDs []EvidenceID
}

type GroundedSynthesisDecision struct {
	Paragraphs []GroundedParagraph
}

type GroundedSynthesisStation interface {
	Synthesize(context.Context, GroundedSynthesisCall) (GroundedSynthesisDecision, error)
}

type GroundedSynthesisCorrectionCall struct {
	Question          string
	Context           assemblyline.ObjectiveContext
	Paragraphs        []GroundedParagraph
	Issue             ClaimEvidenceReviewDecision
	Evidence          []ProjectedEvidence
	MaxParagraphBytes int
}

type GroundedSynthesisCorrectionDecision struct {
	Text string
}

type GroundedSynthesisCorrectionStation interface {
	Correct(context.Context, GroundedSynthesisCorrectionCall) (GroundedSynthesisCorrectionDecision, error)
}

type ParagraphID string
type ClaimEvidenceReviewOutcome string
type ClaimEvidenceIssueKind string

const (
	ClaimEvidenceReviewNone  ClaimEvidenceReviewOutcome = "none"
	ClaimEvidenceReviewIssue ClaimEvidenceReviewOutcome = "issue"

	ClaimEvidenceInsufficientSupport ClaimEvidenceIssueKind = "insufficient_support"
	ClaimEvidenceContradictedSupport ClaimEvidenceIssueKind = "contradicted_support"
	ClaimEvidenceQuestionMismatch    ClaimEvidenceIssueKind = "question_mismatch"
)

type ClaimEvidenceReviewCall struct {
	Question      string
	Context       assemblyline.ObjectiveContext
	ParagraphID   ParagraphID
	ParagraphText string
	Evidence      []ProjectedEvidence
}

type ClaimEvidenceReviewDecision struct {
	Outcome     ClaimEvidenceReviewOutcome
	ParagraphID ParagraphID
	EvidenceIDs []EvidenceID
	IssueKind   ClaimEvidenceIssueKind
	Detail      string
}

type ClaimEvidenceReviewStation interface {
	Review(context.Context, ClaimEvidenceReviewCall) (ClaimEvidenceReviewDecision, error)
}

// Acquisition is code-operated web mechanics. It is deliberately not exposed
// to any station and has no model-visible operation or action representation.
type Acquisition interface {
	Limits() websearch.AcquisitionLimits
	Discover(context.Context, websearch.QueryRequest) (websearch.CandidateReport, error)
	Fetch(context.Context, websearch.FetchRequest) (websearch.DocumentReport, error)
}
