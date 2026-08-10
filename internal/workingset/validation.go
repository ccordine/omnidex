package workingset

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/taskstate"
)

func validateBudget(budget Budget) error {
	if budget.MaxItems <= 0 || budget.MaxBytes <= 0 {
		return fmt.Errorf("%w: resident item and byte limits must be positive", ErrInvalidBudget)
	}
	if budget.MaxPinnedItems < 0 || budget.MaxPinnedItems > budget.MaxItems {
		return fmt.Errorf("%w: pinned item limit must be between zero and max items", ErrInvalidBudget)
	}
	if budget.MaxPinnedBytes < 0 || budget.MaxPinnedBytes > budget.MaxBytes {
		return fmt.Errorf("%w: pinned byte limit must be between zero and max bytes", ErrInvalidBudget)
	}
	if budget.MaxItems > MaxHistoricalItems || budget.MaxBytes > MaxResidentBytes {
		return fmt.Errorf(
			"%w: resident limits exceed the %d-item or %d-byte hard ceiling",
			ErrInvalidBudget, MaxHistoricalItems, MaxResidentBytes,
		)
	}
	return nil
}

func validateScope(scope Scope) error {
	switch scope.Kind {
	case ScopeCall, ScopeStep, ScopePhase, ScopeTask, ScopeObjective, ScopeJob:
	default:
		return fmt.Errorf("%w: scope kind %q is not registered", ErrInvalidScope, scope.Kind)
	}
	if err := requireExactIdentity(string(scope.ID), "scope ID", ErrInvalidScope); err != nil {
		return err
	}
	return nil
}

func validateReference(ref Ref) error {
	if err := taskstate.ValidateRef(ref); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidReference, err)
	}
	if len(ref.URI) > MaxReferenceURIBytes || len(ref.Version) > MaxReferenceVersionBytes {
		return fmt.Errorf("%w: reference URI or version exceeds the persistence limit", ErrInvalidReference)
	}
	return nil
}

func validateAcquire(request AcquireRequest) error {
	if err := requireExactIdentity(string(request.ID), "item ID", ErrInvalidItem); err != nil {
		return err
	}
	if err := validateReference(request.Ref); err != nil {
		return err
	}
	if err := validateRole(request.Role); err != nil {
		return err
	}
	if request.Priority < 1 || request.Priority > 100 {
		return fmt.Errorf("%w: priority must be between 1 and 100", ErrInvalidItem)
	}
	if request.ByteCost <= 0 || request.ByteCost > MaxResidentBytes {
		return fmt.Errorf("%w: byte cost must be between 1 and %d", ErrInvalidItem, MaxResidentBytes)
	}
	if err := validateScope(request.Scope); err != nil {
		return err
	}
	if err := validateMembership(request.Scope, request.Retention); err != nil {
		return err
	}
	return validateAcquisition(request.Acquisition)
}

func validateReacquire(request ReacquireRequest) error {
	if err := requireExactIdentity(string(request.ItemID), "item ID", ErrInvalidItem); err != nil {
		return err
	}
	if err := validateReference(request.Ref); err != nil {
		return err
	}
	if err := validateMembership(request.Scope, request.Retention); err != nil {
		return err
	}
	if request.ExpectedReacquisitionCount >= uint64(math.MaxInt64) {
		return fmt.Errorf("%w: reacquisition count exceeds PostgreSQL BIGINT", ErrInvalidItem)
	}
	return requireExact(request.Reason, "reacquisition reason", ErrInvalidItem)
}

func validateRole(role Role) error {
	switch role {
	case RoleUserAuthority, RoleGoal, RoleObjective, RoleTask, RoleAcceptanceCriterion,
		RoleConstraint, RoleFact, RoleHypothesis, RoleDecision, RoleInvariant, RoleFailure, RoleQuestion, RoleEvidence,
		RoleRepositoryEvidence, RoleDependency, RoleVerification, RoleHistorical:
		return nil
	default:
		return fmt.Errorf("%w: role %q is not registered", ErrInvalidItem, role)
	}
}

func validateAcquisition(acquisition Acquisition) error {
	switch acquisition.Provider {
	case ProviderUser, ProviderTaskState, ProviderRepository, ProviderArtifact,
		ProviderEvidence, ProviderDurableMemory, ProviderWeb, ProviderCompiler,
		ProviderTest, ProviderCommand:
	default:
		return fmt.Errorf("%w: provider %q is not registered", ErrInvalidAcquisition, acquisition.Provider)
	}
	if err := requireExactIdentity(acquisition.OperationID, "acquisition operation ID", ErrInvalidAcquisition); err != nil {
		return err
	}
	return requireExact(acquisition.Reason, "acquisition reason", ErrInvalidAcquisition)
}

func validateMembership(scope Scope, retention Retention) error {
	if err := validateScope(scope); err != nil {
		return err
	}
	if retention == RetentionPinned {
		return nil
	}
	if string(scope.Kind) != string(retention) {
		return fmt.Errorf("%w: %s retention requires a %s scope", ErrInvalidRetention, retention, retention)
	}
	return nil
}

func requireExact(value, field string, sentinel error) error {
	return requireExactBounded(value, field, MaxExactReasonBytes, sentinel)
}

func requireExactIdentity(value, field string, sentinel error) error {
	return requireExactBounded(value, field, MaxExactIdentityBytes, sentinel)
}

func requireExactBounded(value, field string, maxBytes int, sentinel error) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		strings.ContainsRune(value, '\x00') || len(value) > maxBytes {
		return fmt.Errorf("%w: %s must be one nonempty exact value", sentinel, field)
	}
	return nil
}

func referenceKey(ref Ref) string {
	return taskstate.RefIdentity(ref)
}

func scopeKey(scope Scope) string { return string(scope.Kind) + "\x00" + string(scope.ID) }

func (set *Set) validateScopeOwnership(scope Scope) error {
	if err := validateScope(scope); err != nil {
		return err
	}
	if scope.Kind == ScopeJob && scope != set.scope {
		return fmt.Errorf("%w: job scope must equal the immutable owner scope", ErrInvalidScope)
	}
	return nil
}
