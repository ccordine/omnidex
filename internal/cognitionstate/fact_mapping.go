package cognitionstate

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

type FactAcceptancePolicyID string

type FactAcceptancePolicyRef struct {
	ID      FactAcceptancePolicyID `json:"id"`
	Version string                 `json:"version"`
	SHA256  string                 `json:"sha256"`
}

// FactEvidence is immutable environment evidence made available to a
// code-owned acceptance policy. Models and queue callers cannot supply or
// rewrite its content.
type FactEvidence struct {
	Ref     cognition.EvidenceRef
	Content string
}

type FactDeriver func([]FactEvidence) (string, error)

// FactAcceptancePolicy is executable code authority registered by an
// environment composition. Its reference is persisted with every accepted
// fact; its derivation is never model-authored.
type FactAcceptancePolicy struct {
	ref    FactAcceptancePolicyRef
	derive FactDeriver
}

func NewFactAcceptancePolicy(
	ref FactAcceptancePolicyRef,
	derive FactDeriver,
) (FactAcceptancePolicy, error) {
	if err := ref.Validate(); err != nil {
		return FactAcceptancePolicy{}, err
	}
	if derive == nil {
		return FactAcceptancePolicy{}, fmt.Errorf("%w: derivation is nil", ErrInvalidPolicy)
	}
	return FactAcceptancePolicy{ref: ref, derive: derive}, nil
}

func (policy FactAcceptancePolicy) Reference() FactAcceptancePolicyRef { return policy.ref }

func (ref FactAcceptancePolicyRef) Validate() error {
	if !validMappingIdentity(string(ref.ID), 128) || !validMappingIdentity(ref.Version, 64) ||
		!validMappingDigest(ref.SHA256) {
		return fmt.Errorf("%w: policy identity, version, or hash is invalid", ErrInvalidPolicy)
	}
	return nil
}

type FactPolicyRegistry struct {
	policies map[FactAcceptancePolicyID]FactAcceptancePolicy
}

func NewFactPolicyRegistry(policies []FactAcceptancePolicy) (FactPolicyRegistry, error) {
	if len(policies) == 0 || len(policies) > 64 {
		return FactPolicyRegistry{}, fmt.Errorf("%w: policy count must be between one and 64", ErrInvalidPolicy)
	}
	registered := make(map[FactAcceptancePolicyID]FactAcceptancePolicy, len(policies))
	for index, policy := range policies {
		ref := policy.ref
		if err := ref.Validate(); err != nil {
			return FactPolicyRegistry{}, fmt.Errorf("policy %d: %w", index, err)
		}
		if policy.derive == nil {
			return FactPolicyRegistry{}, fmt.Errorf("policy %d: %w: derivation is nil", index, ErrInvalidPolicy)
		}
		if _, duplicate := registered[ref.ID]; duplicate {
			return FactPolicyRegistry{}, fmt.Errorf("%w: policy ID %q is duplicated", ErrInvalidPolicy, ref.ID)
		}
		registered[ref.ID] = policy
	}
	return FactPolicyRegistry{policies: registered}, nil
}

type FactAcceptanceInput struct {
	Ledger       taskstate.MaterializedState
	ScopeNodeID  taskstate.NodeID
	EvidenceRefs []cognition.EvidenceRef
	PolicyID     FactAcceptancePolicyID
}

func (registry FactPolicyRegistry) MapAcceptedFact(input FactAcceptanceInput) (EntryMutation, error) {
	policy, exists := registry.policies[input.PolicyID]
	if !exists {
		return EntryMutation{}, fmt.Errorf("%w: %q", ErrPolicyNotRegistered, input.PolicyID)
	}
	if len(input.EvidenceRefs) == 0 || len(input.EvidenceRefs) > cognition.MaxEvidenceRefs {
		return EntryMutation{}, fmt.Errorf("%w: accepted fact requires bounded evidence", ErrImmutableEvidence)
	}
	if err := taskstate.ValidateMaterializedState(input.Ledger); err != nil {
		return EntryMutation{}, fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	entries := toolObservationMaterials(input.Ledger.Entries)
	refs := make([]taskstate.Ref, len(input.EvidenceRefs))
	evidence := make([]FactEvidence, len(input.EvidenceRefs))
	seen := make(map[cognition.EvidenceRef]struct{}, len(input.EvidenceRefs))
	for index, ref := range input.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return EntryMutation{}, fmt.Errorf("%w: evidence %d: %v", ErrImmutableEvidence, index, err)
		}
		if _, duplicate := seen[ref]; duplicate {
			return EntryMutation{}, fmt.Errorf("%w: evidence %d is duplicated", ErrImmutableEvidence, index)
		}
		seen[ref] = struct{}{}
		refs[index] = evidenceLedgerRef(ref)
		material, exists := entries[taskstate.RefIdentity(refs[index])]
		if !exists || material.Hash != refs[index].Hash {
			return EntryMutation{}, fmt.Errorf("%w: evidence %d has no active tool observation", ErrImmutableEvidence, index)
		}
		evidence[index] = FactEvidence{Ref: ref, Content: material.Content}
	}
	sort.Slice(evidence, func(left, right int) bool {
		return evidenceIdentity(evidence[left].Ref) < evidenceIdentity(evidence[right].Ref)
	})
	for index := range evidence {
		refs[index] = evidenceLedgerRef(evidence[index].Ref)
	}
	content, err := policy.derive(append([]FactEvidence{}, evidence...))
	if err != nil {
		return EntryMutation{}, fmt.Errorf("%w: policy %q: %v", ErrFactPolicyRejected, policy.ref.ID, err)
	}
	mutation, err := newEntryMutation(entryCommandInput{
		Ledger: input.Ledger, ScopeNodeID: input.ScopeNodeID,
		SourceKind: SourceAcceptedFact,
		Source: struct {
			Content  string                  `json:"content"`
			Evidence []cognition.EvidenceRef `json:"evidence"`
			Policy   FactAcceptancePolicyRef `json:"policy"`
		}{content, factEvidenceRefs(evidence), policy.ref},
		Actor: taskstate.AuthorityCode, Kind: taskstate.EntryFact,
		Content: content, Refs: refs,
		Metadata:        map[string]any{"acceptance_policy": policy.ref},
		ExpectedVersion: input.Ledger.Version,
	})
	if err != nil {
		return EntryMutation{}, err
	}
	if err := validateEntryMutation(input.Ledger, mutation); err != nil {
		return EntryMutation{}, err
	}
	return mutation, nil
}

type toolObservationMaterial struct {
	Hash    string
	Content string
}

func toolObservationMaterials(entries []taskstate.Entry) map[string]toolObservationMaterial {
	result := make(map[string]toolObservationMaterial)
	for _, entry := range entries {
		if entry.Status != taskstate.EntryActive || entry.Kind != taskstate.EntryObservation ||
			entry.Authority != taskstate.AuthorityToolEvidence {
			continue
		}
		for _, ref := range entry.Refs {
			if ref.Relation == taskstate.RefEvidence && ref.Hash == entry.ContentSHA256 {
				result[taskstate.RefIdentity(ref)] = toolObservationMaterial{
					Hash: ref.Hash, Content: entry.Content,
				}
			}
		}
	}
	return result
}

func toolObservationEvidence(entries []taskstate.Entry) map[string]string {
	result := make(map[string]string)
	for identity, material := range toolObservationMaterials(entries) {
		result[identity] = material.Hash
	}
	return result
}

func factEvidenceRefs(evidence []FactEvidence) []cognition.EvidenceRef {
	refs := make([]cognition.EvidenceRef, len(evidence))
	for index := range evidence {
		refs[index] = evidence[index].Ref
	}
	return refs
}

func evidenceIdentity(ref cognition.EvidenceRef) string {
	ledgerRef := evidenceLedgerRef(ref)
	return taskstate.RefIdentity(ledgerRef)
}
