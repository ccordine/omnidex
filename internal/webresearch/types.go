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
	ID            EvidenceID
	CandidateID   websearch.CandidateID
	DocumentID    websearch.DocumentID
	URL           string
	Title         string
	Snippet       string
	Content       string
	ContentSHA256 string
	ObservedAt    time.Time
	Truncated     bool
}

type Step string

const (
	StepInitialDiscovery   Step = "initial_discovery"
	StepDocumentsFetched   Step = "documents_fetched"
	StepRelevanceResolved  Step = "relevance_resolved"
	StepEvidenceProjected  Step = "evidence_projected"
)

type CitationSource struct {
	Number        int
	EvidenceID    EvidenceID
	CandidateID   websearch.CandidateID
	DocumentID    websearch.DocumentID
	Title         string
	URL           string
	ContentSHA256 string
	ObservedAt    time.Time
	Truncated     bool
}

type Artifact struct {
	Paragraphs []GroundedParagraph
	Sources    []CitationSource
	Rendered   string
	SHA256     string
}

type evidenceRun struct {
	Objective               Objective
	Steps                   []Step
	Discovery               []websearch.CandidateReport
	Fetches                 []websearch.DocumentReport
	Evidence                []Evidence
	Projected               []ProjectedEvidence
	AcquisitionAttempts     int
	AcquisitionAttemptLimit int
	DiscoveryAttempts       int
	FetchAttempts           int
	RelevanceCalls          int
	SemanticCalls           int
	CallLedger              SemanticCallLedger
}
