package codingagent

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// Public aliases keep the provider boundary on the normalized model rather than
// introducing a second ledger representation.
type EngineeringLedger = model.EngineeringLedger
type LedgerItem = model.LedgerItem
type LedgerObjective = model.LedgerObjective
type LedgerRequirement = model.LedgerRequirement
type LedgerConstraint = model.LedgerConstraint
type LedgerGroup = model.LedgerGroup
type LedgerTask = model.LedgerTask
type LedgerAcceptanceCriterion = model.LedgerAcceptanceCriterion
type LedgerCheck = model.LedgerCheck
type LedgerDependency = model.LedgerDependency
type LedgerMutation = model.LedgerMutation

type RouteSet struct {
	Planner  Route `json:"planner"`
	Executor Route `json:"executor"`
	Reviewer Route `json:"reviewer"`
}

func (routes RouteSet) Validate() error {
	for _, candidate := range []struct {
		name  string
		route Route
	}{
		{name: "planner", route: routes.Planner},
		{name: "executor", route: routes.Executor},
		{name: "reviewer", route: routes.Reviewer},
	} {
		if err := validateRoute(candidate.route); err != nil {
			return fmt.Errorf("%s route: %w", candidate.name, err)
		}
	}
	return nil
}

type RouteConfigurationState string

const (
	RouteConfigurationProposed RouteConfigurationState = "proposed"
	RouteConfigurationFrozen   RouteConfigurationState = "frozen"
)

// RouteConfigurationRevision is immutable configuration history. A later route
// change names the exact prior revision and remains attached to the same ledger.
type RouteConfigurationRevision struct {
	ID             string                  `json:"id"`
	Revision       int64                   `json:"revision"`
	BaseRevision   int64                   `json:"base_revision"`
	LedgerID       string                  `json:"ledger_id"`
	LedgerRevision int64                   `json:"ledger_revision"`
	OwnerTurnID    string                  `json:"owner_turn_id"`
	State          RouteConfigurationState `json:"state"`
	Routes         RouteSet                `json:"routes"`
}

func (configuration RouteConfigurationRevision) Validate() error {
	if strings.TrimSpace(configuration.ID) == "" || configuration.Revision < 1 ||
		configuration.BaseRevision < 0 || configuration.BaseRevision != configuration.Revision-1 ||
		strings.TrimSpace(configuration.LedgerID) == "" || configuration.LedgerRevision < 0 {
		return fmt.Errorf("route configuration requires identity and exact configuration and ledger revisions")
	}
	switch configuration.State {
	case RouteConfigurationProposed, RouteConfigurationFrozen:
	default:
		return fmt.Errorf("route configuration has invalid state %q", configuration.State)
	}
	if configuration.Revision > 1 && strings.TrimSpace(configuration.OwnerTurnID) == "" {
		return fmt.Errorf("route configuration revision requires its owner turn")
	}
	if err := configuration.Routes.Validate(); err != nil {
		return err
	}
	if configuration.State == RouteConfigurationFrozen {
		return configuration.Routes.validateFrozen()
	}
	return nil
}

func (routes RouteSet) validateFrozen() error {
	for _, candidate := range []struct {
		name  string
		route Route
	}{
		{name: "planner", route: routes.Planner},
		{name: "executor", route: routes.Executor},
		{name: "reviewer", route: routes.Reviewer},
	} {
		if candidate.route.Effort == "" || candidate.route.ServiceTier == "" {
			return fmt.Errorf("%s route requires explicit reasoning effort and service tier when frozen", candidate.name)
		}
	}
	return nil
}

type ExecutionConfigurationRevision = RouteConfigurationRevision

func ValidateLedgerMutation(currentRevision int64, mutation LedgerMutation) error {
	if mutation.BaseRevision != currentRevision {
		return fmt.Errorf("ledger mutation base revision %d is stale; current revision is %d", mutation.BaseRevision, currentRevision)
	}
	return mutation.Validate()
}

func validateRoute(route Route) error {
	if strings.TrimSpace(route.Provider) == "" || strings.TrimSpace(route.Model) == "" {
		return fmt.Errorf("provider and model are required")
	}
	for _, option := range []string{route.Effort, route.ServiceTier} {
		if len(option) > 64 || strings.TrimSpace(option) != option || strings.ContainsAny(option, "\x00\r\n") {
			return fmt.Errorf("inference option must be an exact supported value")
		}
	}
	return nil
}
