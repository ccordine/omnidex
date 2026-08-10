package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
)

const cognitionAcceptedFactSchemaV1 = "omnidex.cognition-accepted-fact.v1"

type cognitionAcceptedFact struct {
	Schema            string                                 `json:"schema"`
	ID                string                                 `json:"id"`
	SHA256            string                                 `json:"sha256"`
	EpisodeID         cognition.EpisodeID                    `json:"episode_id"`
	LedgerID          taskstate.LedgerID                     `json:"ledger_id"`
	TransitionID      string                                 `json:"transition_id"`
	TransitionSHA256  string                                 `json:"transition_sha256"`
	ScopeObligationID cognition.ObligationID                 `json:"scope_obligation_id"`
	AuthoritySHA256   string                                 `json:"authority_sha256"`
	Planner           cognitionstate.FactPlannerRef          `json:"planner"`
	Policy            cognitionstate.FactAcceptancePolicyRef `json:"policy"`
	EvidenceRefs      []cognition.EvidenceRef                `json:"evidence_refs"`
	Mapping           cognitionstate.ReplayDescriptor        `json:"mapping"`
}

func (value cognitionAcceptedFact) identity() any {
	return struct {
		Schema            string                                 `json:"schema"`
		EpisodeID         cognition.EpisodeID                    `json:"episode_id"`
		LedgerID          taskstate.LedgerID                     `json:"ledger_id"`
		TransitionID      string                                 `json:"transition_id"`
		TransitionSHA256  string                                 `json:"transition_sha256"`
		ScopeObligationID cognition.ObligationID                 `json:"scope_obligation_id"`
		AuthoritySHA256   string                                 `json:"authority_sha256"`
		Planner           cognitionstate.FactPlannerRef          `json:"planner"`
		Policy            cognitionstate.FactAcceptancePolicyRef `json:"policy"`
		EvidenceRefs      []cognition.EvidenceRef                `json:"evidence_refs"`
		Mapping           cognitionstate.ReplayDescriptor        `json:"mapping"`
	}{value.Schema, value.EpisodeID, value.LedgerID, value.TransitionID,
		value.TransitionSHA256, value.ScopeObligationID, value.AuthoritySHA256,
		value.Planner, value.Policy, append([]cognition.EvidenceRef{}, value.EvidenceRefs...), value.Mapping}
}

func (value cognitionAcceptedFact) validate() error {
	_, digest, err := cognitionJSON(value.identity())
	if err != nil || value.Schema != cognitionAcceptedFactSchemaV1 ||
		value.ID != "cognition_accepted_fact_"+value.SHA256 || value.SHA256 != digest ||
		value.EpisodeID == "" || value.LedgerID == "" || value.TransitionID == "" ||
		!cognitionDigestPattern.MatchString(value.TransitionSHA256) || value.ScopeObligationID == "" ||
		!cognitionDigestPattern.MatchString(value.AuthoritySHA256) || value.Planner.Validate() != nil ||
		value.Policy.Validate() != nil || value.EvidenceRefs == nil || len(value.EvidenceRefs) == 0 ||
		value.Mapping.SourceKind != cognitionstate.SourceAcceptedFact ||
		value.Mapping.EntryID == "" || value.Mapping.CommandID == "" {
		return fmt.Errorf("%w: accepted cognition fact is invalid", ErrCognitionConflict)
	}
	for _, ref := range value.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("%w: accepted cognition fact evidence: %v", ErrCognitionConflict, err)
		}
	}
	return nil
}
