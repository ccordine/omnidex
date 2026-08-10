package cognitionstate

import "sort"

func (registry FactPolicyRegistry) references() []FactAcceptancePolicyRef {
	refs := make([]FactAcceptancePolicyRef, 0, len(registry.policies))
	for _, policy := range registry.policies {
		refs = append(refs, policy.Reference())
	}
	sort.Slice(refs, func(left, right int) bool { return factPolicyRefLess(refs[left], refs[right]) })
	return refs
}

func factPolicyRefLess(left, right FactAcceptancePolicyRef) bool {
	if left.ID != right.ID {
		return left.ID < right.ID
	}
	if left.Version != right.Version {
		return left.Version < right.Version
	}
	return left.SHA256 < right.SHA256
}
