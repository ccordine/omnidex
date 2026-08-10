package cognition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func (decision CognitionDecision) Validate(schema ActionSchema) error {
	if err := validateIdentity(string(decision.ObligationID), "obligation ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDecision, err)
	}
	if err := schema.ValidateRequest(decision.Action, decision.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: action: %v", ErrInvalidDecision, err)
	}
	if err := validateExactText(decision.ExpectedEffect, "expected effect", MaxExpectedEffectBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDecision, err)
	}
	if len(decision.Proposals) > MaxLedgerProposals {
		return fmt.Errorf("%w: ledger proposal count exceeds %d", ErrInvalidDecision, MaxLedgerProposals)
	}
	if len(decision.Attention) > MaxAttentionRequests {
		return fmt.Errorf("%w: attention request count exceeds %d", ErrInvalidDecision, MaxAttentionRequests)
	}
	available := make(map[string]struct{}, len(decision.EvidenceRefs))
	for _, ref := range decision.EvidenceRefs {
		available[evidenceIdentity(ref)] = struct{}{}
	}
	if err := validateLedgerProposals(decision.Proposals, available); err != nil {
		return err
	}
	return validateAttentionRequests(decision.Attention, available)
}

func validateLedgerProposals(proposals []LedgerProposal, available map[string]struct{}) error {
	seen := make(map[string]struct{}, len(proposals))
	obligations := 0
	revisions := 0
	planRevisions := 0
	for index, proposal := range proposals {
		switch proposal.Kind {
		case ProposalObservation, ProposalHypothesis, ProposalQuestion:
			if proposal.Obligation != nil || proposal.Revision != nil || proposal.PlanRevision != nil {
				return fmt.Errorf("%w: proposal %d carries a typed payload under kind %q", ErrInvalidDecision, index, proposal.Kind)
			}
			if err := validateExactText(proposal.Content, "ledger proposal content", MaxProposalBytes); err != nil {
				return fmt.Errorf("%w: proposal %d: %v", ErrInvalidDecision, index, err)
			}
		case ProposalObligation:
			obligations++
			if obligations > 1 {
				return fmt.Errorf("%w: a decision may propose at most one obligation", ErrInvalidDecision)
			}
			if proposal.Content != "" || len(proposal.EvidenceRefs) != 0 || proposal.Obligation == nil ||
				proposal.Revision != nil || proposal.PlanRevision != nil {
				return fmt.Errorf("%w: proposal %d obligation semantics are ambiguous", ErrInvalidDecision, index)
			}
			if err := proposal.Obligation.Validate(); err != nil {
				return fmt.Errorf("%w: proposal %d: %v", ErrInvalidDecision, index, err)
			}
		case ProposalRevision:
			revisions++
			if revisions > 1 || len(proposals) != 1 {
				return fmt.Errorf("%w: a belief revision must be the decision's sole ledger proposal", ErrInvalidDecision)
			}
			if proposal.Content != "" || len(proposal.EvidenceRefs) != 0 || proposal.Obligation != nil ||
				proposal.Revision == nil || proposal.PlanRevision != nil {
				return fmt.Errorf("%w: proposal %d revision semantics are ambiguous", ErrInvalidDecision, index)
			}
			if err := proposal.Revision.Validate(); err != nil {
				return fmt.Errorf("%w: proposal %d: %v", ErrInvalidDecision, index, err)
			}
		case ProposalPlanRevision:
			planRevisions++
			if planRevisions > 1 || len(proposals) != 1 {
				return fmt.Errorf("%w: a plan revision must be the decision's sole ledger proposal", ErrInvalidDecision)
			}
			if proposal.Content != "" || len(proposal.EvidenceRefs) != 0 || proposal.Obligation != nil ||
				proposal.Revision != nil || proposal.PlanRevision == nil {
				return fmt.Errorf("%w: proposal %d plan revision semantics are ambiguous", ErrInvalidDecision, index)
			}
			if err := proposal.PlanRevision.Validate(); err != nil {
				return fmt.Errorf("%w: proposal %d: %v", ErrInvalidDecision, index, err)
			}
		case LedgerProposalKind("fact"), LedgerProposalKind("decision"), LedgerProposalKind("completion"):
			return fmt.Errorf("%w: proposal %d requests authoritative state %q", ErrAuthorityDenied, index, proposal.Kind)
		default:
			return fmt.Errorf("%w: proposal %d kind %q is not registered", ErrInvalidDecision, index, proposal.Kind)
		}
		refs := proposal.EvidenceRefs
		if proposal.Obligation != nil {
			refs = proposal.Obligation.EvidenceRefs
		} else if proposal.Revision != nil {
			refs = proposal.Revision.EvidenceRefs
		} else if proposal.PlanRevision != nil {
			refs = proposal.PlanRevision.EvidenceRefs
		}
		if err := validateEvidenceRefs(refs); err != nil {
			return fmt.Errorf("%w: proposal %d: %v", ErrInvalidDecision, index, err)
		}
		if proposal.Kind != ProposalQuestion && len(refs) == 0 {
			return fmt.Errorf("%w: proposal %d requires supporting evidence", ErrInvalidDecision, index)
		}
		if err := requireAvailableEvidence(refs, available, "ledger proposal"); err != nil {
			return err
		}
		key := string(proposal.Kind) + "\x00" + proposal.Content
		if proposal.Obligation != nil {
			var err error
			key, err = obligationProposalSHA256(*proposal.Obligation)
			if err != nil {
				return fmt.Errorf("%w: proposal %d identity: %v", ErrInvalidDecision, index, err)
			}
		} else if proposal.Revision != nil {
			key = string(proposal.Kind) + "\x00" + proposal.Revision.TargetRef.URI + "\x00" +
				proposal.Revision.TargetRef.Version + "\x00" + proposal.Revision.TargetRef.SHA256
		} else if proposal.PlanRevision != nil {
			var err error
			key, err = planRevisionProposalSHA256(*proposal.PlanRevision)
			if err != nil {
				return fmt.Errorf("%w: proposal %d identity: %v", ErrInvalidDecision, index, err)
			}
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: proposal %d is duplicated", ErrInvalidDecision, index)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateAttentionRequests(requests []AttentionRequest, available map[string]struct{}) error {
	seen := make(map[string]struct{}, len(requests))
	for index, request := range requests {
		switch request.Operation {
		case AttentionRetain, AttentionRelease:
		default:
			return fmt.Errorf("%w: attention request %d operation %q is not registered", ErrInvalidDecision, index, request.Operation)
		}
		switch request.Scope {
		case AttentionScopeDecision, AttentionScopeObligation, AttentionScopeEpisode:
		default:
			return fmt.Errorf("%w: attention request %d scope %q is not registered", ErrInvalidDecision, index, request.Scope)
		}
		if err := request.TargetRef.Validate(); err != nil {
			return fmt.Errorf("%w: attention request %d: %v", ErrInvalidDecision, index, err)
		}
		if err := requireAvailableEvidence([]EvidenceRef{request.TargetRef}, available, "attention request"); err != nil {
			return err
		}
		if err := validateExactText(request.Reason, "attention reason", MaxAttentionReasonBytes); err != nil {
			return fmt.Errorf("%w: attention request %d: %v", ErrInvalidDecision, index, err)
		}
		key := string(request.Operation) + "\x00" + string(request.Scope) + "\x00" + evidenceIdentity(request.TargetRef)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: attention request %d is duplicated", ErrInvalidDecision, index)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func requireAvailableEvidence(refs []EvidenceRef, available map[string]struct{}, field string) error {
	for _, ref := range refs {
		if _, exists := available[evidenceIdentity(ref)]; !exists {
			return fmt.Errorf("%w: %s cites evidence outside the decision packet", ErrInvalidDecision, field)
		}
	}
	return nil
}

func DecodeCognitionDecision(raw []byte, schema ActionSchema) (CognitionDecision, error) {
	if err := ValidateCognitionDecisionAuthority(raw); err != nil {
		return CognitionDecision{}, err
	}
	if err := ValidateExactJSONFields(raw, CognitionDecision{}, "decision"); err != nil {
		return CognitionDecision{}, fmt.Errorf("%w: decode decision object: %v", ErrInvalidDecision, err)
	}
	if err := validateDecisionJSONContract(raw); err != nil {
		return CognitionDecision{}, fmt.Errorf("%w: decode decision contract: %v", ErrInvalidDecision, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var decision CognitionDecision
	if err := decoder.Decode(&decision); err != nil {
		return CognitionDecision{}, fmt.Errorf("%w: decode decision: %v", ErrInvalidDecision, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return CognitionDecision{}, fmt.Errorf("%w: decision must contain exactly one JSON object", ErrInvalidDecision)
	}
	if err := decision.Validate(schema); err != nil {
		return CognitionDecision{}, err
	}
	return decision.Clone(), nil
}

// ValidateCognitionDecisionAuthority rejects model attempts to claim code-owned
// completion before an action schema is selected. It intentionally does not
// validate the rest of the decision contract.
func ValidateCognitionDecisionAuthority(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxDecisionBytes || !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return fmt.Errorf("%w: decision JSON is empty, oversized, or invalid text", ErrInvalidDecision)
	}
	if err := ValidateUniqueJSONObject(raw, "decision"); err != nil {
		return fmt.Errorf("%w: decode decision object: %v", ErrInvalidDecision, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%w: decode decision object: %v", ErrInvalidDecision, err)
	}
	for field := range fields {
		for _, forbidden := range []string{"complete", "completed", "completion", "goal_complete", "terminal"} {
			if strings.EqualFold(field, forbidden) {
				return fmt.Errorf("%w: models cannot declare completion", ErrAuthorityDenied)
			}
		}
	}
	return nil
}

func (decision CognitionDecision) Clone() CognitionDecision {
	decision.Action = decision.Action.Clone()
	decision.EvidenceRefs = cloneSlice(decision.EvidenceRefs)
	decision.Proposals = cloneLedgerProposals(decision.Proposals)
	decision.Attention = cloneSlice(decision.Attention)
	return decision
}

func cloneLedgerProposals(proposals []LedgerProposal) []LedgerProposal {
	if proposals == nil {
		return nil
	}
	cloned := make([]LedgerProposal, len(proposals))
	for index, proposal := range proposals {
		proposal.EvidenceRefs = cloneSlice(proposal.EvidenceRefs)
		if proposal.Obligation != nil {
			cloned := proposal.Obligation.Clone()
			proposal.Obligation = &cloned
		}
		if proposal.Revision != nil {
			cloned := proposal.Revision.Clone()
			proposal.Revision = &cloned
		}
		if proposal.PlanRevision != nil {
			cloned := proposal.PlanRevision.Clone()
			proposal.PlanRevision = &cloned
		}
		cloned[index] = proposal
	}
	return cloned
}
