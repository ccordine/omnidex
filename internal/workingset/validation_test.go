package workingset

import (
	"errors"
	"testing"
)

func TestWorkingSetRejectsInvalidBudgetsAndScopes(t *testing.T) {
	t.Parallel()
	validOwner := testOwner(t, 1, 1)
	tests := []struct {
		name   string
		owner  Owner
		budget Budget
		want   error
	}{
		{name: "empty owner", budget: Budget{MaxItems: 1, MaxBytes: 1}, want: ErrInvalidSet},
		{name: "zero items", owner: validOwner, budget: Budget{MaxBytes: 1}, want: ErrInvalidBudget},
		{name: "zero bytes", owner: validOwner, budget: Budget{MaxItems: 1}, want: ErrInvalidBudget},
		{name: "too many pins", owner: validOwner, budget: Budget{MaxItems: 1, MaxBytes: 1, MaxPinnedItems: 2}, want: ErrInvalidBudget},
		{name: "too many pin bytes", owner: validOwner, budget: Budget{MaxItems: 1, MaxBytes: 1, MaxPinnedBytes: 2}, want: ErrInvalidBudget},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.owner, test.budget); !errors.Is(err, test.want) {
				t.Fatalf("New error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestAcquireValidatesEveryBoundedFieldAndDuplicateIdentity(t *testing.T) {
	t.Parallel()
	scope := Scope{Kind: ScopeCall, ID: "call-1"}
	base := testRequest("item-1", "repo://snapshot/file/one", "d", scope, RetentionCall, 5)
	tests := []struct {
		name   string
		mutate func(*AcquireRequest)
		want   error
	}{
		{name: "empty item id", mutate: func(r *AcquireRequest) { r.ID = "" }, want: ErrInvalidItem},
		{name: "unknown role", mutate: func(r *AcquireRequest) { r.Role = "unknown" }, want: ErrInvalidItem},
		{name: "zero priority", mutate: func(r *AcquireRequest) { r.Priority = 0 }, want: ErrInvalidItem},
		{name: "zero bytes", mutate: func(r *AcquireRequest) { r.ByteCost = 0 }, want: ErrInvalidItem},
		{name: "bad ref hash", mutate: func(r *AcquireRequest) { r.Ref.Hash = "bad" }, want: ErrInvalidReference},
		{name: "unknown provider", mutate: func(r *AcquireRequest) { r.Acquisition.Provider = "unknown" }, want: ErrInvalidAcquisition},
		{name: "blank reason", mutate: func(r *AcquireRequest) { r.Acquisition.Reason = " " }, want: ErrInvalidAcquisition},
		{name: "unknown scope", mutate: func(r *AcquireRequest) { r.Scope.Kind = "unknown" }, want: ErrInvalidScope},
		{name: "retention mismatch", mutate: func(r *AcquireRequest) { r.Retention = RetentionTask }, want: ErrInvalidRetention},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 20, MaxPinnedItems: 1, MaxPinnedBytes: 10})
			request := base
			test.mutate(&request)
			if _, err := set.Acquire(request); !errors.Is(err, test.want) {
				t.Fatalf("Acquire error = %v, want %v", err, test.want)
			}
			if set.Version() != 0 || len(set.Items()) != 0 {
				t.Fatalf("invalid acquisition mutated set: version=%d items=%#v", set.Version(), set.Items())
			}
		})
	}

	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 20, MaxPinnedItems: 1, MaxPinnedBytes: 10})
	if _, err := set.Acquire(base); err != nil {
		t.Fatalf("acquire valid item: %v", err)
	}
	duplicate := base
	duplicate.ID = "item-2"
	if _, err := set.Acquire(duplicate); !errors.Is(err, ErrDuplicateReference) {
		t.Fatalf("duplicate reference error = %v, want ErrDuplicateReference", err)
	}
	conflict := duplicate
	conflict.ID = "item-3"
	conflict.Ref.Hash = repeatDigest("f")
	if _, err := set.Acquire(conflict); !errors.Is(err, ErrReferenceConflict) {
		t.Fatalf("conflicting reference error = %v, want ErrReferenceConflict", err)
	}
}

func TestAcquireUsesTaskStateSchemeSuffixReferenceGrammar(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 10})
	request := testRequest("item", "repo:snapshot/abc/symbol/one", "a", Scope{Kind: ScopeCall, ID: "call-1"}, RetentionCall, 5)
	if _, err := set.Acquire(request); err != nil {
		t.Fatalf("acquire canonical scheme:suffix reference: %v", err)
	}
}

func TestWorkingSetRegistersStructuredTaskAttentionRoles(t *testing.T) {
	t.Parallel()
	roles := []Role{
		RoleUserAuthority,
		RoleGoal,
		RoleObjective,
		RoleTask,
		RoleAcceptanceCriterion,
		RoleConstraint,
		RoleFact,
		RoleHypothesis,
		RoleDecision,
		RoleInvariant,
		RoleFailure,
		RoleQuestion,
		RoleEvidence,
		RoleRepositoryEvidence,
		RoleDependency,
		RoleVerification,
		RoleHistorical,
	}
	for _, role := range roles {
		if err := validateRole(role); err != nil {
			t.Fatalf("role %q is not registered: %v", role, err)
		}
	}
}

func TestReturnedItemsCannotMutateWorkingSetState(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 20, MaxPinnedItems: 1, MaxPinnedBytes: 10})
	scope := Scope{Kind: ScopeObjective, ID: "objective-1"}
	acquireTestItem(t, set, "item", "repo://snapshot/symbol/one", "e", scope, RetentionObjective, 5)

	item, _ := set.Item("item")
	item.Memberships[0].Scope.ID = "tampered"
	items := set.Items()
	items[0].Memberships = nil

	current, _ := set.Item("item")
	if len(current.Memberships) != 1 || current.Memberships[0].Scope.ID != "objective-1" {
		t.Fatalf("returned item leaked mutable state: %#v", current)
	}
}
