package labyrinth

import (
	"github.com/gryph/omnidex/internal/cognition"
)

const (
	GeneratedScenarioSchemaV1 = "labyrinth.generated-scenario.v1"
	PublicWorldSchemaV1       = "labyrinth.public-world.v1"
	OracleSchemaV1            = "labyrinth.private-oracle.v1"
)

type GeneratedScenario struct {
	Schema   string                `json:"schema"`
	Scenario cognition.ScenarioRef `json:"scenario"`
	World    PublicWorld           `json:"world"`
}

type OracleQuality string

const (
	OracleOptimal     OracleQuality = "optimal"
	OracleWitnessOnly OracleQuality = "witness_only"
)

type TaskArchetype string

const (
	ArchetypeRetrieve TaskArchetype = "bounded-retrieval"
	ArchetypeRecall   TaskArchetype = "delayed-evidence-recall"
	ArchetypeUnlock   TaskArchetype = "causal-prerequisite-unlock"
	ArchetypeMutate   TaskArchetype = "evidence-bound-mutation"
	ArchetypeCombined TaskArchetype = "combined-prerequisite-mutation"
)

type EvidenceIdentity struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

// EvidenceUse binds one exact public record to the witness action that
// acquires it and the strictly later action whose legality depends on it.
// It is private evaluator authority and is never part of GeneratedScenario.
type EvidenceUse struct {
	Evidence            EvidenceIdentity   `json:"evidence"`
	AcquisitionActionID cognition.ActionID `json:"acquisition_action_id"`
	RequiredByActionID  cognition.ActionID `json:"required_by_action_id"`
}

type CausalEdge struct {
	From EntityID `json:"from"`
	To   EntityID `json:"to"`
}

type WitnessAction struct {
	ID      cognition.ActionID        `json:"id"`
	Schema  cognition.ActionSchemaRef `json:"schema"`
	Request cognition.ActionRequest   `json:"request"`
	Cost    int                       `json:"cost"`
}

type Oracle struct {
	Schema           string               `json:"schema"`
	ScenarioID       cognition.ScenarioID `json:"scenario_id"`
	PublicSHA256     string               `json:"public_sha256"`
	OracleSHA256     string               `json:"oracle_sha256"`
	GeneratorVersion string               `json:"generator_version"`
	GrammarVersion   string               `json:"grammar_version"`
	Seed             uint64               `json:"seed"`
	DefinitionSHA256 string               `json:"definition_sha256"`
	Quality          OracleQuality        `json:"quality"`
	Witness          []WitnessAction      `json:"witness"`
	WitnessCost      int                  `json:"witness_cost"`
	OptimalCost      *int                 `json:"optimal_cost,omitempty"`
	LowerBound       int                  `json:"lower_bound"`
	RequiredEvidence []EvidenceIdentity   `json:"required_evidence"`
	EvidenceUses     []EvidenceUse        `json:"evidence_uses"`
	CausalDAG        []CausalEdge         `json:"causal_dag"`
	TaskArchetype    TaskArchetype        `json:"task_archetype"`
	ExpandedStates   int                  `json:"expanded_states"`
}

// GeneratedCase keeps execution, public, and private authorities separate.
// It deliberately cannot be serialized as one aggregate artifact.
type GeneratedCase struct {
	execution Scenario
	public    GeneratedScenario
	oracle    Oracle
}
