package contextcompiler

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	maxStationAttempts = 2

	// MinModelCalls is zero because an explicit code-owned retrieval directive
	// can prove that no semantic lookup or reduction is required. Each invoked
	// station retains its own bounded correction budget; the total number of
	// relevance and reduction calls is derived from acquired authority volume.
	MinModelCalls = 0
)

// RetrievalDirective is code-owned authority for an exact retrieval request.
// A nil directive means query formulation remains semantically unresolved. A
// non-nil directive, including one with an empty Concepts slice, forbids a
// ceremonial search-term model call.
type RetrievalDirective struct {
	Concepts []string
}

type Request struct {
	ExactInstruction string
	Retrieval        *RetrievalDirective
}

// OptionalSelectionGroup binds contiguous optional candidate chunks that must
// remain one code-owned selection unit. The relationship is never projected to
// a semantic station; stations continue to receive only ordinary candidate
// authorities and return only opaque candidate IDs.
type OptionalSelectionGroup struct {
	CandidateIDs []string
}

type CandidateSet struct {
	Required                []assemblyline.ContextCandidateAuthority
	Optional                []assemblyline.ContextCandidateAuthority
	OptionalSelectionGroups []OptionalSelectionGroup
	Replan                  *assemblyline.ObjectiveReplanAuthority
}

// SearchAvailability is code-owned authority for whether term-directed
// retrieval can add candidates beyond the provider's mechanically acquired
// required and optional candidates. Unknown availability is invalid: callers
// may not guess whether a semantic search-term question exists.
type SearchAvailability string

const (
	SearchUnavailable SearchAvailability = "unavailable"
	SearchAvailable   SearchAvailability = "available"
)

func (availability SearchAvailability) Validate() error {
	switch availability {
	case SearchUnavailable, SearchAvailable:
		return nil
	default:
		return fmt.Errorf("context search availability %q is invalid", availability)
	}
}

type CandidateProvider interface {
	SearchAvailability(context.Context) (SearchAvailability, error)
	Retrieve(context.Context, []string) (CandidateSet, error)
}

type StationReceipt struct {
	Calls int
}

type SearchTermsStation interface {
	Generate(context.Context, assemblyline.ContextSearchTermsInput) (
		assemblyline.ContextSearchTermsDecision, StationReceipt, error,
	)
}

type RelevanceStation interface {
	SelectRelevant(context.Context, assemblyline.ContextRelevanceInput) (
		assemblyline.ContextRelevanceDecision, StationReceipt, error,
	)
}

type MinificationStation interface {
	Minify(context.Context, assemblyline.ContextMinificationInput) (
		assemblyline.ContextMinificationDecision, StationReceipt, error,
	)
}

type Stations struct {
	Terms        SearchTermsStation
	Relevance    RelevanceStation
	Minification MinificationStation
}

type Result struct {
	Context           assemblyline.ObjectiveContext
	SearchTermsCalls  int
	RelevanceCalls    int
	MinificationCalls int
	ModelCalls        int
}
