package contextcompiler

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	// MinModelCalls is zero because an explicit code-owned retrieval directive
	// can prove that no semantic lookup or reduction is required. Each invoked
	// station call resolves exactly one raw leaf; the total number of relevance
	// and reduction calls is derived from acquired authority volume.
	MinModelCalls = 0
)

// RetrievalDirective is code-owned authority for whether the fixed provider
// can search beyond mechanically retained candidates. The query itself is
// always Request.ExactInstruction and cannot be supplied independently.
// A nil directive asks Compile to inspect the provider's fixed availability.
type RetrievalDirective struct {
	Availability SearchAvailability
}

type Request struct {
	ExactInstruction   string
	ModelInstruction   string
	Retrieval          *RetrievalDirective
	Scope              assemblyline.ContextScope
	KnownArtifactPaths []string
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

// SearchAvailability is code-owned authority for whether exact-instruction
// retrieval can add candidates beyond the provider's mechanically acquired
// required and optional candidates. Unknown availability is invalid: callers
// may not guess whether another deterministic retrieval is available.
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
	Calls  int
	Reused bool
}

type RelevanceStation interface {
	Relate(context.Context, assemblyline.ContextRelevanceRelationInput) (
		assemblyline.ContextRelevanceRelationResult, StationReceipt, error,
	)
}

type MinificationStation interface {
	Minify(context.Context, assemblyline.ContextMinificationInput) (
		assemblyline.ContextMinificationDecision, StationReceipt, error,
	)
}

type Stations struct {
	Relevance    RelevanceStation
	Minification MinificationStation
}

type Result struct {
	Context           assemblyline.ObjectiveContext
	RelevanceCalls    int
	MinificationCalls int
	ModelCalls        int
}
