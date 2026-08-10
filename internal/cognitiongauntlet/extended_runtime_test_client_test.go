package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/taskstate"
)

type extendedWitnessPolicyClient struct {
	*witnessPolicyClient
	suite labyrinth.Suite
}

type extendedWitnessEnvelope struct {
	ActionCatalog    cognition.ActionCatalog `json:"action_catalog"`
	EvidenceRefs     []cognition.EvidenceRef `json:"evidence_refs"`
	ProjectedContext struct {
		Items []struct {
			Ref     taskstate.Ref `json:"ref"`
			Role    string        `json:"role"`
			Content string        `json:"content"`
		} `json:"items"`
	} `json:"projected_context"`
}

func (*extendedWitnessPolicyClient) ExtendedEvidenceClass() ExtendedEvidenceClass {
	return ExtendedEvidenceStructuralWitness
}

func (client *extendedWitnessPolicyClient) GeneratePrepared(
	ctx context.Context,
	prepared llm.PreparedModel,
) (string, error) {
	if client.suite != labyrinth.SuiteRevise && client.suite != labyrinth.SuiteRogue {
		return client.witnessPolicyClient.GeneratePrepared(ctx, prepared)
	}
	if err := client.ValidateExactPreparedContract(prepared); err != nil {
		return "", err
	}
	return client.generateRevisionDecision(prepared.BaseModel, prepared.Prompt)
}

func (client *extendedWitnessPolicyClient) generateRevisionDecision(
	modelName, prompt string,
) (string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if modelName != client.model || client.next >= len(client.witness) {
		return "", fmt.Errorf("extended witness policy call is outside its sealed witness")
	}
	var envelope extendedWitnessEnvelope
	if err := json.Unmarshal([]byte(prompt), &envelope); err != nil {
		return "", err
	}
	current, err := extendedCurrentObligation(envelope)
	if err != nil {
		return "", err
	}
	witness := client.witness[client.next]
	schema, exists := envelope.ActionCatalog.Schema(witness.Request.Kind)
	if !exists {
		return "", fmt.Errorf("extended witness action %q is unavailable", witness.Request.Kind)
	}
	if _, err := client.captureAcquiredEvidence(envelope.EvidenceRefs); err != nil {
		return "", err
	}
	evidence := append([]cognition.EvidenceRef{}, client.consumerEvidence(witness.ID)...)
	proposals, attention, additional, err := client.revisionProposals(envelope, witness)
	if err != nil {
		return "", err
	}
	evidence = appendUniqueEvidence(evidence, additional)
	if evidence == nil {
		evidence = []cognition.EvidenceRef{}
	}
	if schema.EvidencePolicy == cognition.EvidenceRequired &&
		(len(evidence) == 0 || !evidenceAvailable(evidence, envelope.EvidenceRefs)) {
		return "", fmt.Errorf("extended witness required evidence is absent")
	}
	decision := cognition.CognitionDecision{
		ObligationID: current.ID, Action: witness.Request.Clone(), EvidenceRefs: evidence,
		ExpectedEffect: "Apply this action.",
		Proposals:      proposals, Attention: attention,
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		return "", err
	}
	client.next++
	return string(raw), nil
}

func (client *extendedWitnessPolicyClient) revisionProposals(
	envelope extendedWitnessEnvelope,
	witness labyrinth.WitnessAction,
) ([]cognition.LedgerProposal, []cognition.AttentionRequest, []cognition.EvidenceRef, error) {
	latest := latestEvidenceRefs(envelope.EvidenceRefs)
	if client.suite == labyrinth.SuiteRogue {
		return client.rogueRevisionProposals(envelope, witness, latest)
	}
	switch client.next {
	case 1:
		if len(latest) == 0 {
			return nil, nil, nil, fmt.Errorf("tentative evidence is absent")
		}
		return []cognition.LedgerProposal{{
			Kind:    cognition.ProposalHypothesis,
			Content: "The other route may be unavailable.", EvidenceRefs: latest,
		}}, retainExtendedEvidence(latest), latest, nil
	case 2:
		target, err := extendedHypothesisRef(envelope)
		if err != nil || len(latest) == 0 {
			return nil, nil, nil, fmt.Errorf("active hypothesis or contradiction evidence is absent")
		}
		return []cognition.LedgerProposal{{
			Kind:     cognition.ProposalRevision,
			Revision: &cognition.BeliefRevisionProposal{TargetRef: target, EvidenceRefs: latest},
		}}, retainExtendedEvidence(latest), latest, nil
	case 4:
		to, ok := actionArgument(witness.Request, "to")
		if !ok || len(latest) == 0 {
			return nil, nil, nil, fmt.Errorf("revised route target or supporting evidence is absent")
		}
		predicate, err := cognition.NewPredicate("surface.marker", []string{to})
		if err != nil {
			return nil, nil, nil, err
		}
		goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		resolved := client.consumerEvidence(client.witness[3].ID)
		return []cognition.LedgerProposal{{
			Kind:         cognition.ProposalPlanRevision,
			PlanRevision: &cognition.PlanRevisionProposal{Next: goal, EvidenceRefs: latest},
		}}, releaseExtendedEvidence(resolved), appendUniqueEvidence(latest, resolved), nil
	default:
		return []cognition.LedgerProposal{}, []cognition.AttentionRequest{}, nil, nil
	}
}

func extendedCurrentObligation(envelope extendedWitnessEnvelope) (cognition.Obligation, error) {
	for _, item := range envelope.ProjectedContext.Items {
		if item.Role != "task" {
			continue
		}
		var obligation cognition.Obligation
		if json.Unmarshal([]byte(item.Content), &obligation) == nil && obligation.ID != "" {
			return obligation, nil
		}
	}
	return cognition.Obligation{}, fmt.Errorf("extended current obligation is absent")
}

func extendedHypothesisRef(envelope extendedWitnessEnvelope) (cognition.EpistemicRef, error) {
	var result cognition.EpistemicRef
	for _, item := range envelope.ProjectedContext.Items {
		if item.Role != "hypothesis" {
			continue
		}
		if result.URI != "" {
			return cognition.EpistemicRef{}, fmt.Errorf("multiple active hypotheses are visible")
		}
		result = cognition.EpistemicRef{
			URI: item.Ref.URI, Version: item.Ref.Version, SHA256: item.Ref.Hash,
		}
	}
	if err := result.Validate(); err != nil {
		return cognition.EpistemicRef{}, err
	}
	return result, nil
}

func retainExtendedEvidence(refs []cognition.EvidenceRef) []cognition.AttentionRequest {
	result := make([]cognition.AttentionRequest, len(refs))
	for index, ref := range refs {
		result[index] = cognition.AttentionRequest{
			Operation: cognition.AttentionRetain, TargetRef: ref,
			Scope:  cognition.AttentionScopeObligation,
			Reason: "This evidence supports the unresolved current decision.",
		}
	}
	return result
}

func releaseExtendedEvidence(refs []cognition.EvidenceRef) []cognition.AttentionRequest {
	result := make([]cognition.AttentionRequest, len(refs))
	for index, ref := range refs {
		result[index] = cognition.AttentionRequest{
			Operation: cognition.AttentionRelease, TargetRef: ref,
			Scope:  cognition.AttentionScopeObligation,
			Reason: "This evidence has completed its current causal role.",
		}
	}
	return result
}

func actionArgument(request cognition.ActionRequest, name cognition.ActionArgumentName) (string, bool) {
	for _, argument := range request.Arguments {
		if argument.Name == name {
			return argument.Value, true
		}
	}
	return "", false
}
