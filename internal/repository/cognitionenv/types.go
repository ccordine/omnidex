package cognitionenv

import (
	"context"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const (
	ScenarioSchemaV2 = "omnidex.repository-cognition-scenario.v2"
	ActionVersionV1  = "1.0.0"

	PredicateEvidenceAcquired cognition.PredicateName   = "repository.evidence_acquired"
	ObservationNeed           cognition.ObservationKind = "repository.investigation_need"
	ObservationEvidence       cognition.ObservationKind = "repository.evidence_pack"
	ObservationState          cognition.ObservationKind = "repository.investigation_state"

	ActionSearch      cognition.ActionKind         = "repository.search"
	ActionInspect     cognition.ActionKind         = "repository.inspect"
	ActionReferences  cognition.ActionKind         = "repository.references"
	ArgumentSymbolRef cognition.ActionArgumentName = "symbol_ref"

	PublicOutcomeEvidenceAcquired = "registered repository evidence acquired"
)

var fixedLimits = repositoryretrieval.Limits{
	MaxSymbols: 8, MaxEdges: 32, MaxSpanBytes: 4 * 1024, MaxPackBytes: 9 * 1024,
}

type investigationStage string

const (
	stageStart     investigationStage = "start"
	stageSearched  investigationStage = "searched"
	stageInspected investigationStage = "inspected"
	stageComplete  investigationStage = "complete"
)

type investigationState struct {
	Schema         string             `json:"schema"`
	Stage          investigationStage `json:"stage"`
	DiscoveredRefs []string           `json:"discovered_symbol_refs"`
	InspectedRefs  []string           `json:"inspected_symbol_refs"`
	EvidencePackID string             `json:"latest_evidence_pack_id"`
}

type EvidenceBuilder interface {
	Build(context.Context, repositoryretrieval.Request) (repositoryretrieval.EvidencePack, error)
}

type AttemptAuthorizer func(context.Context, cognition.AttemptRef) error

type NeedAuthority struct {
	ID            string `json:"id"`
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
}

// Investigation binds one immutable repository snapshot and analysis to one
// accepted retrieval need. Paths and plaintext query authority have no public
// getters and are never rendered by this environment.
type Investigation struct {
	projectID    int64
	snapshot     repositoryfacts.Snapshot
	analysis     repositoryfacts.Analysis
	operation    repositoryretrieval.Operation
	query        string
	queryBinding string
	need         NeedAuthority
	ref          cognition.ScenarioRef
	goal         cognition.GoalExpression
	catalog      cognition.ActionCatalog
	completion   cognition.CompletionAuthority
}

func (value Investigation) Ref() cognition.ScenarioRef       { return value.ref }
func (value Investigation) Goal() cognition.GoalExpression   { return value.goal.Clone() }
func (value Investigation) Catalog() cognition.ActionCatalog { return value.catalog.Clone() }
func (value Investigation) Completion() cognition.CompletionAuthority {
	return value.completion.Clone()
}

func (value Investigation) SnapshotID() string { return value.snapshot.ID }
func (value Investigation) AnalysisID() string { return value.analysis.ID }
func (value Investigation) Operation() repositoryretrieval.Operation {
	return value.operation
}
