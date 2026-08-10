package cognitionstate

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const FactAcceptanceAuthoritySchemaV1 = "omnidex.cognition-fact-acceptance-authority.v1"

type FactPlannerRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

func (ref FactPlannerRef) Validate() error {
	if !validMappingIdentity(ref.ID, 128) || !validMappingIdentity(ref.Version, 64) ||
		!validMappingDigest(ref.SHA256) {
		return fmt.Errorf("%w: planner identity, version, or hash is invalid", ErrInvalidPolicy)
	}
	return nil
}

type FactPlan struct {
	PolicyID     FactAcceptancePolicyID  `json:"policy_id"`
	EvidenceRefs []cognition.EvidenceRef `json:"evidence_refs"`
}

type FactPlanner interface {
	Reference() FactPlannerRef
	Plan(cognition.Transition) ([]FactPlan, error)
}

type FactAcceptanceAuthorityRef struct {
	Schema   string                    `json:"schema"`
	Planner  FactPlannerRef            `json:"planner"`
	Policies []FactAcceptancePolicyRef `json:"policies"`
	SHA256   string                    `json:"sha256"`
}

func (ref FactAcceptanceAuthorityRef) Validate() error {
	if ref.Schema != FactAcceptanceAuthoritySchemaV1 || ref.Planner.Validate() != nil ||
		ref.Policies == nil || !validMappingDigest(ref.SHA256) {
		return fmt.Errorf("%w: fact acceptance authority reference is invalid", ErrInvalidPolicy)
	}
	for index, policy := range ref.Policies {
		if policy.Validate() != nil || (index > 0 && !factPolicyRefLess(ref.Policies[index-1], policy)) {
			return fmt.Errorf("%w: fact policy references are invalid or unsorted", ErrInvalidPolicy)
		}
	}
	digest, err := mappingDigest(ref.identity())
	if err != nil || digest != ref.SHA256 {
		return fmt.Errorf("%w: fact acceptance authority hash is invalid", ErrInvalidPolicy)
	}
	return nil
}

func (ref FactAcceptanceAuthorityRef) identity() any {
	return struct {
		Schema   string                    `json:"schema"`
		Planner  FactPlannerRef            `json:"planner"`
		Policies []FactAcceptancePolicyRef `json:"policies"`
	}{ref.Schema, ref.Planner, append([]FactAcceptancePolicyRef{}, ref.Policies...)}
}

type FactAcceptanceAuthority struct {
	ref      FactAcceptanceAuthorityRef
	planner  FactPlanner
	registry FactPolicyRegistry
}

func NewFactAcceptanceAuthority(
	planner FactPlanner,
	registry FactPolicyRegistry,
) (FactAcceptanceAuthority, error) {
	if factPlannerNil(planner) {
		return FactAcceptanceAuthority{}, fmt.Errorf("%w: fact planner is nil", ErrInvalidPolicy)
	}
	if err := planner.Reference().Validate(); err != nil {
		return FactAcceptanceAuthority{}, err
	}
	refs := registry.references()
	if len(refs) == 0 {
		return FactAcceptanceAuthority{}, fmt.Errorf("%w: nonempty fact authority requires registered policies", ErrInvalidPolicy)
	}
	return newFactAcceptanceAuthority(planner, registry, refs)
}

func newFactAcceptanceAuthority(
	planner FactPlanner,
	registry FactPolicyRegistry,
	refs []FactAcceptancePolicyRef,
) (FactAcceptanceAuthority, error) {
	ref := FactAcceptanceAuthorityRef{
		Schema: FactAcceptanceAuthoritySchemaV1, Planner: planner.Reference(),
		Policies: append([]FactAcceptancePolicyRef{}, refs...),
	}
	digest, err := mappingDigest(ref.identity())
	if err != nil {
		return FactAcceptanceAuthority{}, err
	}
	ref.SHA256 = digest
	value := FactAcceptanceAuthority{ref: ref, planner: planner, registry: registry}
	if err := value.validate(); err != nil {
		return FactAcceptanceAuthority{}, err
	}
	return value, nil
}

func (authority FactAcceptanceAuthority) Reference() FactAcceptanceAuthorityRef {
	ref := authority.ref
	ref.Policies = append([]FactAcceptancePolicyRef{}, ref.Policies...)
	return ref
}

func (authority FactAcceptanceAuthority) validate() error {
	if factPlannerNil(authority.planner) || authority.ref.Validate() != nil ||
		authority.planner.Reference() != authority.ref.Planner ||
		!reflect.DeepEqual(authority.registry.references(), authority.ref.Policies) {
		return fmt.Errorf("%w: executable fact authority differs from its reference", ErrInvalidPolicy)
	}
	return nil
}

func (authority FactAcceptanceAuthority) Validate() error { return authority.validate() }

func (authority FactAcceptanceAuthority) MapTransitionFacts(
	state taskstate.MaterializedState,
	scope taskstate.NodeID,
	transition cognition.Transition,
) ([]EntryMutation, error) {
	if err := authority.validate(); err != nil {
		return nil, err
	}
	plans, err := authority.planner.Plan(transition.Clone())
	if err != nil {
		return nil, fmt.Errorf("%w: fact planner: %v", ErrFactPolicyRejected, err)
	}
	if plans == nil {
		return nil, fmt.Errorf("%w: fact planner returned a null plan", ErrInvalidPolicy)
	}
	plans, err = authority.normalizePlans(transition, plans)
	if err != nil {
		return nil, err
	}
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		return nil, fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	mutations := make([]EntryMutation, 0, len(plans))
	for _, plan := range plans {
		mutation, err := authority.registry.MapAcceptedFact(FactAcceptanceInput{
			Ledger: ledger.MaterializedState(), ScopeNodeID: scope,
			EvidenceRefs: plan.EvidenceRefs, PolicyID: plan.PolicyID,
		})
		if err != nil {
			return nil, err
		}
		if _, err := ledger.Apply(mutation.Command()); err != nil {
			return nil, fmt.Errorf("%w: apply accepted fact: %v", ErrInvalidMapping, err)
		}
		mutations = append(mutations, mutation)
	}
	return mutations, nil
}

func (authority FactAcceptanceAuthority) normalizePlans(
	transition cognition.Transition,
	plans []FactPlan,
) ([]FactPlan, error) {
	available := make(map[cognition.EvidenceRef]struct{}, len(transition.Observations))
	for _, observation := range transition.Observations {
		available[observation.EvidenceRef()] = struct{}{}
	}
	normalized := make([]FactPlan, len(plans))
	for index, plan := range plans {
		if _, exists := authority.registry.policies[plan.PolicyID]; !exists {
			return nil, fmt.Errorf("%w: fact plan %d policy %q", ErrPolicyNotRegistered, index, plan.PolicyID)
		}
		if plan.EvidenceRefs == nil || len(plan.EvidenceRefs) == 0 || len(plan.EvidenceRefs) > cognition.MaxEvidenceRefs {
			return nil, fmt.Errorf("%w: fact plan %d has invalid evidence", ErrImmutableEvidence, index)
		}
		normalized[index] = FactPlan{PolicyID: plan.PolicyID, EvidenceRefs: append([]cognition.EvidenceRef{}, plan.EvidenceRefs...)}
		sort.Slice(normalized[index].EvidenceRefs, func(left, right int) bool {
			return evidenceIdentity(normalized[index].EvidenceRefs[left]) < evidenceIdentity(normalized[index].EvidenceRefs[right])
		})
		for refIndex, ref := range normalized[index].EvidenceRefs {
			if _, exists := available[ref]; !exists ||
				(refIndex > 0 && normalized[index].EvidenceRefs[refIndex-1] == ref) {
				return nil, fmt.Errorf("%w: fact plan %d evidence %d is unavailable or duplicated", ErrImmutableEvidence, index, refIndex)
			}
		}
	}
	sort.Slice(normalized, func(left, right int) bool { return factPlanKey(normalized[left]) < factPlanKey(normalized[right]) })
	for index := 1; index < len(normalized); index++ {
		if factPlanKey(normalized[index-1]) == factPlanKey(normalized[index]) {
			return nil, fmt.Errorf("%w: fact plan is duplicated", ErrInvalidPolicy)
		}
	}
	return normalized, nil
}

func factPlanKey(plan FactPlan) string {
	parts := make([]string, len(plan.EvidenceRefs))
	for index, ref := range plan.EvidenceRefs {
		parts[index] = evidenceIdentity(ref)
	}
	return string(plan.PolicyID) + "\x00" + strings.Join(parts, "\x00")
}

func factPlannerNil(planner FactPlanner) bool {
	if planner == nil {
		return true
	}
	value := reflect.ValueOf(planner)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
