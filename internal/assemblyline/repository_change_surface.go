package assemblyline

import (
	"fmt"
	"strings"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const (
	RepositoryChangeSurfaceSchemaV2 = "omnidex.repository-change-surface.v2"
	maxRepositoryChangeTargets      = 8
	maxRepositoryChangeNeedBytes    = 4 * 1024
	maxRepositoryRequirementBytes   = 512
)

type RepositoryChangeSurfaceInput struct {
	ResearchNeed string                           `json:"research_need"`
	Requirements []string                         `json:"requirements"`
	Evidence     repositoryretrieval.EvidencePack `json:"evidence"`
}

type RepositoryChangeTarget struct {
	SymbolID    string `json:"symbol_id"`
	Requirement string `json:"requirement"`
}

type RepositoryChangeSurfaceDecision struct {
	Schema  string                   `json:"schema"`
	Targets []RepositoryChangeTarget `json:"targets"`
}

func (input RepositoryChangeSurfaceInput) validate() error {
	if input.ResearchNeed == "" || input.ResearchNeed != strings.TrimSpace(input.ResearchNeed) {
		return fmt.Errorf("repository change surface requires one trimmed research need")
	}
	if len(input.ResearchNeed) > maxRepositoryChangeNeedBytes {
		return fmt.Errorf("repository change surface research need exceeds %d bytes", maxRepositoryChangeNeedBytes)
	}
	if len(input.Requirements) == 0 || len(input.Requirements) > maxRepositoryChangeTargets {
		return fmt.Errorf(
			"repository change surface requires 1-%d code-owned requirements",
			maxRepositoryChangeTargets,
		)
	}
	seenRequirements := make(map[string]struct{}, len(input.Requirements))
	for _, requirement := range input.Requirements {
		if err := validateRepositoryChangeRequirement(requirement); err != nil {
			return err
		}
		if _, duplicate := seenRequirements[requirement]; duplicate {
			return fmt.Errorf("repository change surface requirements must be unique")
		}
		seenRequirements[requirement] = struct{}{}
	}
	if err := input.Evidence.Validate(); err != nil {
		return fmt.Errorf("repository change surface evidence: %w", err)
	}
	return nil
}

func (decision RepositoryChangeSurfaceDecision) ValidateFor(input RepositoryChangeSurfaceInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RepositoryChangeSurfaceSchemaV2 {
		return fmt.Errorf("repository change surface schema must be %q", RepositoryChangeSurfaceSchemaV2)
	}
	if len(decision.Targets) > maxRepositoryChangeTargets {
		return fmt.Errorf("repository change surface exceeds %d bounded targets", maxRepositoryChangeTargets)
	}
	available := make(map[string]struct{}, len(input.Evidence.Symbols))
	for _, symbol := range input.Evidence.Symbols {
		available[symbol.ID] = struct{}{}
	}
	omittedSource := make(map[string]struct{}, len(input.Evidence.SourceOmissions))
	for _, omission := range input.Evidence.SourceOmissions {
		omittedSource[omission.SymbolID] = struct{}{}
	}
	targetIDs := make(map[string]struct{}, len(decision.Targets))
	required := make(map[string]struct{}, len(input.Requirements))
	for _, requirement := range input.Requirements {
		required[requirement] = struct{}{}
	}
	for _, target := range decision.Targets {
		if _, exists := available[target.SymbolID]; !exists {
			return fmt.Errorf("repository change target %q is absent from evidence", target.SymbolID)
		}
		if _, omitted := omittedSource[target.SymbolID]; omitted {
			return fmt.Errorf("repository change target %q has no bounded source evidence", target.SymbolID)
		}
		if _, duplicate := targetIDs[target.SymbolID]; duplicate {
			return fmt.Errorf("repository change target symbols must be unique")
		}
		targetIDs[target.SymbolID] = struct{}{}
		if err := validateRepositoryChangeRequirement(target.Requirement); err != nil {
			return err
		}
		if _, registered := required[target.Requirement]; !registered {
			return fmt.Errorf("repository change target uses an unregistered requirement")
		}
	}
	return nil
}

func (decision RepositoryChangeSurfaceDecision) UnresolvedRequirements(
	input RepositoryChangeSurfaceInput,
) ([]string, error) {
	if err := decision.ValidateFor(input); err != nil {
		return nil, err
	}
	resolved := make(map[string]struct{}, len(decision.Targets))
	for _, target := range decision.Targets {
		resolved[target.Requirement] = struct{}{}
	}
	unresolved := make([]string, 0, len(input.Requirements))
	for _, requirement := range input.Requirements {
		if _, exists := resolved[requirement]; !exists {
			unresolved = append(unresolved, requirement)
		}
	}
	return unresolved, nil
}

func validateRepositoryChangeRequirement(requirement string) error {
	if requirement == "" || requirement != strings.TrimSpace(requirement) ||
		len([]byte(requirement)) > maxRepositoryRequirementBytes {
		return fmt.Errorf(
			"repository change surface requires trimmed requirements of at most %d bytes",
			maxRepositoryRequirementBytes,
		)
	}
	return nil
}
