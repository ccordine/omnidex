package webresearch

import (
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

var (
	ErrInvalidObjective           = errors.New("invalid web research objective")
	ErrInvalidConfiguration       = errors.New("invalid web research configuration")
	ErrInvalidAcquisition         = errors.New("invalid web research acquisition")
	ErrEvidenceUnavailable        = errors.New("web research evidence unavailable")
	ErrInvalidSearchTerms         = errors.New("invalid web search terms decision")
	ErrInvalidRelevance           = errors.New("invalid web relevance decision")
	ErrInvalidSynthesis           = errors.New("invalid grounded synthesis decision")
	ErrInvalidSynthesisCorrection = errors.New("invalid grounded synthesis correction decision")
	ErrInvalidClaimEvidenceReview = errors.New("invalid claim-evidence review decision")
	ErrClaimEvidenceInadequate    = errors.New("web research claim evidence inadequate")
	ErrNilContext                 = errors.New("web research context is nil")
)

type ObjectiveID string
type ObjectiveStatus string

const (
	ObjectivePending  ObjectiveStatus = "pending"
	ObjectiveComplete ObjectiveStatus = "complete"
)

type AcceptancePredicate string

const (
	AcceptanceGroundedSynthesis   AcceptancePredicate = "grounded_synthesis_validated"
	AcceptanceExactCitations      AcceptancePredicate = "exact_citations_validated"
	AcceptanceClaimEvidenceReview AcceptancePredicate = "claim_evidence_reviewed"
)

type Objective struct {
	ID           ObjectiveID
	Question     string
	Context      assemblyline.ObjectiveContext
	InitialQuery string
	Acceptance   []AcceptancePredicate
	Status       ObjectiveStatus
}

type Config struct {
	MaxSearchTerms             int
	MaxSearchTermBytes         int
	MaxFetchCandidates         int
	MaxProjectionBytes         int
	MaxRelevantCandidates      int
	CandidateSummaryBytes      int
	MaxSynthesisParagraphs     int
	MaxSynthesisParagraphBytes int
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
	StepInitialDiscovery      Step = "initial_discovery"
	StepSearchTermsResolved   Step = "search_terms_resolved"
	StepExpandedDiscovery     Step = "expanded_discovery"
	StepDocumentsFetched      Step = "documents_fetched"
	StepRelevanceResolved     Step = "relevance_resolved"
	StepEvidenceProjected     Step = "evidence_projected"
	StepSynthesisResolved     Step = "synthesis_resolved"
	StepSynthesisCorrected    Step = "synthesis_corrected"
	StepSynthesisZeroDelta    Step = "synthesis_correction_zero_delta"
	StepClaimEvidenceReviewed Step = "claim_evidence_reviewed"
	StepObjectiveCompleted    Step = "objective_completed"
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

type Result struct {
	Objective                     Objective
	Steps                         []Step
	Discovery                     []websearch.CandidateReport
	Fetches                       []websearch.DocumentReport
	Evidence                      []Evidence
	Projected                     []ProjectedEvidence
	Artifact                      Artifact
	AcquisitionAttempts           int
	AcquisitionAttemptLimit       int
	DiscoveryAttempts             int
	FetchAttempts                 int
	SearchTermsCalls              int
	RelevanceCalls                int
	SynthesisCalls                int
	SynthesisCorrectionCalls      int
	SynthesisCorrectionZeroDeltas int
	ClaimEvidenceReviewCalls      int
	Complete                      bool
}
