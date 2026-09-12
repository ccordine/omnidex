package webresearch

import (
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

var (
	ErrInvalidObjective     = errors.New("invalid web research objective")
	ErrInvalidConfiguration = errors.New("invalid web research configuration")
	ErrInvalidAcquisition   = errors.New("invalid web research acquisition")
	ErrEvidenceUnavailable  = errors.New("web research evidence unavailable")
	ErrInvalidRelevance     = errors.New("invalid web relevance decision")
	ErrInvalidSynthesis     = errors.New("invalid grounded synthesis decision")
	ErrNilContext           = errors.New("web research context is nil")
)

type ObjectiveID string
type ObjectiveStatus string

const (
	ObjectivePending ObjectiveStatus = "pending"
)

type Objective struct {
	ID                 ObjectiveID
	Question           string
	Context            assemblyline.ObjectiveContext
	InitialQuery       string
	KnownArtifactPaths []string
	Status             ObjectiveStatus
}

type EvidenceID string

type Evidence struct {
	ID          EvidenceID
	CandidateID websearch.CandidateID
	URL         string
	Title       string
	Snippet     string
	Content     string
	ObservedAt  time.Time
	Truncated   bool
}

type CitationSource struct {
	Number      int
	EvidenceID  EvidenceID
	CandidateID websearch.CandidateID
	Title       string
	URL         string
	ObservedAt  time.Time
	Truncated   bool
}

type Artifact struct {
	Paragraphs []GroundedParagraph
	Sources    []CitationSource
	Rendered   string
}

type evidenceRun struct {
	Discovery     []websearch.CandidateReport
	Fetches       []websearch.DocumentReport
	Evidence      []Evidence
	Projected     []ProjectedEvidence
	SemanticCalls int
}
