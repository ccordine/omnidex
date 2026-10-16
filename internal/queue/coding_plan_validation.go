package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

var (
	ErrCodingLedgerRevision   = errors.New("coding ledger revision conflict")
	ErrCodingLedgerState      = errors.New("coding ledger state conflict")
	codingLedgerItemIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$`)
)

func validateCodingLedgerRoutes(routes []CodingLedgerRoute, revision int64) error {
	if len(routes) != 3 {
		return fmt.Errorf("coding ledger requires exactly planner, executor, and reviewer routes")
	}
	seen := map[string]bool{}
	var configurationRevision int64
	for _, route := range routes {
		if route.Role != "planner" && route.Role != "executor" && route.Role != "reviewer" {
			return fmt.Errorf("coding ledger route has unsupported role %q", route.Role)
		}
		if seen[route.Role] {
			return fmt.Errorf("coding ledger route repeats role %q", route.Role)
		}
		seen[route.Role] = true
		if route.ConfigurationRevision <= 0 || route.ConfigurationRevision > revision {
			return fmt.Errorf("coding ledger route %s has invalid configuration revision", route.Role)
		}
		if configurationRevision == 0 {
			configurationRevision = route.ConfigurationRevision
		} else if configurationRevision != route.ConfigurationRevision {
			return fmt.Errorf("coding ledger routes require one configuration revision")
		}
		for _, value := range []string{route.Provider, route.Model, route.ReasoningEffort, route.ServiceTier} {
			if value == "" || len(value) > 64 || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("coding ledger route %s contains an invalid option", route.Role)
			}
		}
	}
	return nil
}

func validateInitialCodingLedgerReviewStates(ledger model.EngineeringLedger) error {
	for _, record := range ledgerRepositoryRecords(ledger) {
		if record.item.Disposition == model.LedgerDispositionActive &&
			record.item.ReviewState != model.LedgerReviewProposed {
			return fmt.Errorf("initial coding ledger item %s must start proposed", record.item.ID)
		}
	}
	for _, dependency := range ledger.Dependencies {
		if dependency.Disposition == model.LedgerDispositionActive &&
			dependency.ReviewState != model.LedgerReviewProposed {
			return fmt.Errorf("initial coding ledger dependency %s must start proposed", dependency.ID)
		}
	}
	return nil
}

func validateCodingLedgerRouteMutation(
	current, supplied []CodingLedgerRoute,
	nextRevision int64,
	operations []model.LedgerMutationOperation,
	disposition model.OwnerTurnDisposition,
) error {
	if len(supplied) == 0 || codingLedgerRoutesEqual(current, supplied) {
		return nil
	}
	represented := false
	for _, statement := range disposition.StatementDispositions() {
		represented = represented || statement.Kind == model.OwnerTurnRepresented
	}
	if len(operations) == 0 || !represented {
		return fmt.Errorf("coding ledger route replacement requires a represented material mutation")
	}
	for _, route := range supplied {
		if route.ConfigurationRevision != nextRevision {
			return fmt.Errorf("coding ledger route replacement requires configuration revision %d", nextRevision)
		}
	}
	return nil
}

func validateLedgerForRepository(ledger model.EngineeringLedger) error {
	if err := ledger.Validate(); err != nil {
		return err
	}
	for _, record := range ledgerRepositoryRecords(ledger) {
		if !codingLedgerItemIDPattern.MatchString(string(record.item.ID)) ||
			strings.HasPrefix(string(record.item.ID), codingLedgerDependencyRecordPrefix) {
			return fmt.Errorf("coding ledger item %q has an invalid or reserved stable identity", record.item.ID)
		}
		if len(record.item.Rationale) > 8192 {
			return fmt.Errorf("coding ledger item %s rationale exceeds 8192 bytes", record.item.ID)
		}
	}
	for _, dependency := range ledger.Dependencies {
		if !codingLedgerItemIDPattern.MatchString(string(dependency.ID)) ||
			strings.HasPrefix(string(dependency.ID), codingLedgerDependencyRecordPrefix) {
			return fmt.Errorf("coding ledger dependency %q has an invalid or reserved stable identity", dependency.ID)
		}
		if len(dependency.Rationale) > 8192 {
			return fmt.Errorf("coding ledger dependency %s rationale exceeds 8192 bytes", dependency.ID)
		}
	}
	return validateCodingLedgerExactDuplicates(ledger)
}

// validateCodingLedgerExactDuplicates is shared by complete-ledger validation
// and every draft mutation. Drafts may be temporarily incomplete, but they may
// never persist two records with identical engineering meaning under different
// stable identities.
func validateCodingLedgerExactDuplicates(ledger model.EngineeringLedger) error {
	// Encoding a normalized map produces a stable key order. Index those
	// canonical payloads so exact-duplicate validation remains linear even
	// though the ledger deliberately has no aggregate item-count limit.
	duplicates := make(map[string]struct{})
	add := func(kind string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var normalized map[string]any
		if err := json.Unmarshal(raw, &normalized); err != nil {
			return err
		}
		for _, field := range []string{"id", "provenance", "rationale", "priority", "rank", "disposition", "review_state"} {
			delete(normalized, field)
		}
		canonical, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		key := kind + "\x00" + string(canonical)
		if _, duplicate := duplicates[key]; duplicate {
			return fmt.Errorf("coding ledger contains exact duplicate %s records", kind)
		}
		duplicates[key] = struct{}{}
		return nil
	}
	for _, value := range ledger.Objectives {
		if err := add("objective", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.Requirements {
		if err := add("requirement", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.Constraints {
		if err := add("constraint", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.Groups {
		if err := add("group", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.Tasks {
		if err := add("task", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.AcceptanceCriteria {
		if err := add("acceptance_criterion", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.Checks {
		if err := add("check", value); err != nil {
			return err
		}
	}
	for _, value := range ledger.Dependencies {
		if err := add("dependency", value); err != nil {
			return err
		}
	}
	return nil
}

func validateDraftLedgerForRepository(ledger model.EngineeringLedger) error {
	return ledger.ValidateDraft()
}

func validateOwnerTurnDisposition(ledger model.EngineeringLedger, disposition model.OwnerTurnDisposition) error {
	if len(disposition.Statements) == 0 {
		if len(ledgerRepositoryRecords(ledger)) == 0 && len(ledger.Dependencies) == 0 {
			return nil
		}
		return fmt.Errorf("owner turn requires statement dispositions")
	}
	active := map[model.LedgerItemID]string{}
	for _, record := range ledgerRepositoryRecords(ledger) {
		if record.item.Disposition == model.LedgerDispositionActive {
			active[record.item.ID] = record.kind
		}
	}
	for _, dependency := range ledger.Dependencies {
		if dependency.Disposition == model.LedgerDispositionActive {
			active[dependency.ID] = "dependency"
		}
	}
	for _, disposition := range disposition.StatementDispositions() {
		switch disposition.Kind {
		case model.OwnerTurnRepresented:
			if strings.TrimSpace(disposition.Reason) != "" {
				return fmt.Errorf("represented owner turn cannot include a reason")
			}
			if len(disposition.ItemIDs) == 0 {
				return fmt.Errorf("represented owner turn requires active item references")
			}
			for _, id := range disposition.ItemIDs {
				if active[id] == "" {
					return fmt.Errorf("represented owner turn references unavailable item %s", id)
				}
			}
		case model.OwnerTurnOutOfScope:
			if len(disposition.ItemIDs) != 0 {
				return fmt.Errorf("out-of-scope owner turn cannot include item references")
			}
			if strings.TrimSpace(disposition.Reason) == "" {
				return fmt.Errorf("out-of-scope owner turn requires a reason")
			}
		default:
			return fmt.Errorf("owner turn requires a disposition")
		}
	}
	return nil
}

func validateMutationOwnerTurnDisposition(
	base model.EngineeringLedger,
	next model.EngineeringLedger,
	operations []model.LedgerMutationOperation,
	disposition model.OwnerTurnDisposition,
) error {
	known := make(map[model.LedgerItemID]string)
	for _, record := range ledgerRepositoryRecords(next) {
		known[record.item.ID] = record.kind
	}
	for _, dependency := range next.Dependencies {
		known[dependency.ID] = "dependency"
	}
	represented := make(map[model.LedgerItemID]bool)
	hasOutOfScope := false
	for _, statement := range disposition.StatementDispositions() {
		switch statement.Kind {
		case model.OwnerTurnRepresented:
			for _, id := range statement.ItemIDs {
				if known[id] == "" {
					return fmt.Errorf("represented owner turn references unavailable item %s", id)
				}
				represented[id] = true
			}
		case model.OwnerTurnOutOfScope:
			hasOutOfScope = true
		}
	}
	for _, operation := range operations {
		target := operation.TargetID
		if operation.Kind == model.LedgerMutationAdd {
			target = ledgerMutationItemID(operation.Item)
		}
		if !represented[target] {
			return fmt.Errorf("owner turn disposition does not reference mutation target %s", target)
		}
	}
	if len(operations) > 0 || hasOutOfScope {
		return nil
	}
	_ = base // retained in the contract for future before/after target checks.
	return fmt.Errorf("owner turn requires a disposition consistent with its mutation")
}

func validateCodingLedgerOperationID(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 ||
		strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("coding ledger operation requires a trimmed identity of at most 256 bytes")
	}
	return nil
}
