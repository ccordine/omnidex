package cognitionstate

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func buildContext(
	set *workingset.Set,
	candidates []attentionCandidate,
	goalRef taskstate.Ref,
	sourceSHA string,
) (contextbuilder.ContextSpec, []contextbuilder.Material, error) {
	if len(candidates) == 0 || len(candidates) > MaxContextItems || candidateBytes(candidates) > MaxContextMaterialBytes {
		return contextbuilder.ContextSpec{}, nil, fmt.Errorf("%w: projected context exceeds fixed caps", ErrReconciliationCapacity)
	}
	mandatoryByRole := make(map[workingset.Role]int)
	totalByRole := make(map[workingset.Role]int)
	authorities := make(map[taskstate.Authority]struct{})
	materials := make([]contextbuilder.Material, 0, len(candidates))
	for _, candidate := range candidates {
		item, exists := itemByRef(set, candidate.ref)
		if !exists || item.State != workingset.ItemResident || item.Ref.Hash != candidate.ref.Hash {
			return contextbuilder.ContextSpec{}, nil, fmt.Errorf("%w: candidate %q is not resident", ErrInvalidReconciliation, candidate.key)
		}
		desiredMemberships, err := candidateMemberships(candidate, set.Scope())
		if err != nil {
			return contextbuilder.ContextSpec{}, nil, err
		}
		for _, membership := range desiredMemberships {
			if !itemHasExactMembership(item, membership) {
				return contextbuilder.ContextSpec{}, nil, fmt.Errorf(
					"%w: candidate %q lacks exact scoped retention", ErrInvalidReconciliation, candidate.key,
				)
			}
		}
		totalByRole[candidate.role]++
		if candidate.mandatory {
			mandatoryByRole[candidate.role]++
		}
		authorities[candidate.authority] = struct{}{}
		materials = append(materials, contextbuilder.Material{
			ItemID: item.ID, CurrentRef: candidate.ref, Authority: candidate.authority,
			SourceRefs: append([]taskstate.Ref{}, candidate.sourceRefs...),
			Content:    candidate.content, ByteCost: len(candidate.content),
		})
	}
	sort.Slice(materials, func(left, right int) bool { return materials[left].ItemID < materials[right].ItemID })
	required, optional := selectorsForRoles(mandatoryByRole, totalByRole)
	allowed := orderedAuthorities(authorities)
	scopeRef := goalRef
	scopeRef.Relation = taskstate.RefConcerns
	spec := contextbuilder.ContextSpec{
		Name: DefaultContextSpecName, Version: DefaultContextSpecVersion,
		ScopeRef: scopeRef, Required: required, Optional: optional,
		AllowedAuthorities: allowed, MaxItems: len(candidates), MaxBytes: MaxContextBytes,
		MaxAcquisitionRounds: 0,
	}
	if _, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: "cognition-" + sourceSHA, Spec: spec, WorkingSet: set, Materials: materials,
	}); err != nil {
		return contextbuilder.ContextSpec{}, nil, fmt.Errorf("%w: context projection: %v", ErrInvalidReconciliation, err)
	}
	return spec, materials, nil
}

func selectorsForRoles(
	mandatory map[workingset.Role]int,
	total map[workingset.Role]int,
) ([]contextbuilder.Selector, []contextbuilder.Selector) {
	roles := []workingset.Role{
		workingset.RoleGoal, workingset.RoleTask, workingset.RoleConstraint,
		workingset.RoleFact, workingset.RoleHypothesis, workingset.RoleDecision,
		workingset.RoleInvariant, workingset.RoleFailure, workingset.RoleQuestion, workingset.RoleEvidence,
		workingset.RoleDependency,
	}
	required := make([]contextbuilder.Selector, 0)
	optional := make([]contextbuilder.Selector, 0)
	for _, role := range roles {
		maximum := total[role]
		if maximum == 0 {
			continue
		}
		selector := contextbuilder.Selector{ID: "cognition-" + string(role), Role: role, MaxItems: maximum}
		if minimum := mandatory[role]; minimum > 0 {
			selector.MinItems = minimum
			required = append(required, selector)
		} else {
			optional = append(optional, selector)
		}
	}
	return required, optional
}

func orderedAuthorities(present map[taskstate.Authority]struct{}) []taskstate.Authority {
	order := []taskstate.Authority{
		taskstate.AuthorityUser, taskstate.AuthorityCode, taskstate.AuthorityToolEvidence,
		taskstate.AuthorityAcceptedModelDecision, taskstate.AuthorityModelProposal,
	}
	result := make([]taskstate.Authority, 0, len(present))
	for _, authority := range order {
		if _, exists := present[authority]; exists {
			result = append(result, authority)
		}
	}
	return result
}
