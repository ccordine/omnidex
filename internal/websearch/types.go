package websearch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

var (
	ErrInvalidConfig      = errors.New("invalid web search configuration")
	ErrInvalidQuery       = errors.New("invalid web search query")
	ErrNoCandidates       = errors.New("web search returned no candidates")
	ErrInvalidFetch       = errors.New("invalid web document fetch")
	ErrNoDocuments        = errors.New("web fetch returned no usable documents")
	ErrNilContext         = errors.New("web search context is nil")
	ErrUnsafeURL          = errors.New("unsafe outbound web URL")
	ErrBoundExceeded      = errors.New("web search value exceeds hard bounds")
	ErrDocumentRedirect   = errors.New("web document redirect is forbidden")
	ErrSearchRedirect     = errors.New("web search provider redirect is forbidden")
	ErrInvalidFetchedText = errors.New("web response contains invalid text")
)

type HostResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type ProviderID string

const (
	ProviderDuckDuckGo ProviderID = "duckduckgo"
	ProviderBrave      ProviderID = "brave"
	ProviderGoogle     ProviderID = "google"
	ProviderReddit     ProviderID = "reddit"
	ProviderYahoo      ProviderID = "yahoo"
)

type Config struct {
	Providers                []ProviderID
	Timeout                  time.Duration
	PerDocumentBytes         int
	TotalDocumentBytes       int
	MaxCandidatesPerProvider int
	MaxCandidates            int
	MaxDocuments             int
	MaxResponseBytes         int64
	HTTPClient               *http.Client
	Resolver                 HostResolver
}

// AcquisitionLimits is the exact deterministic fetch authority exposed to
// the research workflow. A workflow may reduce this bound, never exceed it.
type AcquisitionLimits struct {
	MaxDocuments int
}

type QueryRequest struct {
	Query string
}

type CandidateID string

type CandidateSource struct {
	Provider  ProviderID
	SearchURL string
	Rank      int
}

type Candidate struct {
	ID      CandidateID
	URL     string
	Title   string
	Snippet string
	Sources []CandidateSource
}

type DiscoveryOutcome string

const (
	DiscoverySucceeded DiscoveryOutcome = "succeeded"
	DiscoveryEmpty     DiscoveryOutcome = "empty"
	DiscoveryFailed    DiscoveryOutcome = "failed"
)

type ProviderDiagnostic struct {
	Provider       ProviderID
	SearchURL      string
	Outcome        DiscoveryOutcome
	CandidateCount int
	Failure        string
}

type CandidateReport struct {
	Query       string
	Candidates  []Candidate
	Diagnostics []ProviderDiagnostic
}

type FetchRequest struct {
	Candidates   []Candidate
	CandidateIDs []CandidateID
}

type DocumentID string

type Document struct {
	ID            DocumentID
	CandidateID   CandidateID
	URL           string
	Title         string
	Snippet       string
	Content       string
	ContentSHA256 string
	ObservedAt    time.Time
	Truncated     bool
}

type FetchOutcome string

const (
	FetchSucceeded FetchOutcome = "succeeded"
	FetchEmpty     FetchOutcome = "empty"
	FetchFailed    FetchOutcome = "failed"
)

type DocumentDiagnostic struct {
	CandidateID CandidateID
	URL         string
	Outcome     FetchOutcome
	Failure     string
}

type DocumentReport struct {
	Documents   []Document
	Diagnostics []DocumentDiagnostic
}
