package assemblyline

import (
	"fmt"
	"strings"
)

const (
	ApplicationEvidenceNeedSchemaV2   = "omnidex.application-evidence-need.v2"
	maxApplicationEvidenceNeedIDBytes = 128
)

type ApplicationEvidenceNeedKind string

const (
	ApplicationEvidenceContextFact ApplicationEvidenceNeedKind = "context_fact"
	ApplicationEvidenceChangeOwner ApplicationEvidenceNeedKind = "change_owner"
)

type ApplicationEvidenceSourceClass string

const ApplicationEvidenceRepository ApplicationEvidenceSourceClass = "repository"

type ApplicationEvidenceCriterion string

const (
	ApplicationEvidenceDirectlyRelevant ApplicationEvidenceCriterion = "directly_relevant_current_evidence"
	ApplicationEvidenceOwningSymbol     ApplicationEvidenceCriterion = "existing_owning_symbol"
)

type ApplicationEvidenceStopCondition string

const (
	ApplicationEvidenceRelevantSelection ApplicationEvidenceStopCondition = "relevant_current_evidence_selected"
	ApplicationEvidenceOwnerResolved     ApplicationEvidenceStopCondition = "existing_owner_resolved"
)

// ApplicationEvidenceNeed is code-owned investigation state. A semantic station
// may supply Question, but it cannot select a provider, operation, or lifecycle
// transition. Code assigns every other field from a registered resolver contract.
type ApplicationEvidenceNeed struct {
	Schema           string                           `json:"schema"`
	ID               string                           `json:"id"`
	Kind             ApplicationEvidenceNeedKind      `json:"kind"`
	Question         string                           `json:"question"`
	WhyItMatters     string                           `json:"why_it_matters"`
	SourceClasses    []ApplicationEvidenceSourceClass `json:"source_classes"`
	RequiredEvidence []ApplicationEvidenceCriterion   `json:"required_evidence"`
	StopCondition    ApplicationEvidenceStopCondition `json:"stop_condition"`
}

type ApplicationContextEvidence struct {
	Value        string `json:"value"`
	SourceID     string `json:"source_id"`
	SourceSHA256 string `json:"source_sha256"`
}

func NewApplicationRepositoryContextNeed(index int, question string) (ApplicationEvidenceNeed, error) {
	need := ApplicationEvidenceNeed{
		Schema:           ApplicationEvidenceNeedSchemaV2,
		ID:               fmt.Sprintf("context_evidence_need_%03d", index),
		Kind:             ApplicationEvidenceContextFact,
		Question:         question,
		WhyItMatters:     "The request cannot be interpreted faithfully until this fact is established.",
		SourceClasses:    []ApplicationEvidenceSourceClass{ApplicationEvidenceRepository},
		RequiredEvidence: []ApplicationEvidenceCriterion{ApplicationEvidenceDirectlyRelevant},
		StopCondition:    ApplicationEvidenceRelevantSelection,
	}
	return need, need.Validate()
}

func NewApplicationRepositoryChangeOwnerNeed(index int, requirement string) (ApplicationEvidenceNeed, error) {
	need := ApplicationEvidenceNeed{
		Schema:           ApplicationEvidenceNeedSchemaV2,
		ID:               fmt.Sprintf("change_owner_need_%03d", index),
		Kind:             ApplicationEvidenceChangeOwner,
		Question:         requirement,
		WhyItMatters:     "Repository mutation cannot be scoped until the current owning symbol is established.",
		SourceClasses:    []ApplicationEvidenceSourceClass{ApplicationEvidenceRepository},
		RequiredEvidence: []ApplicationEvidenceCriterion{ApplicationEvidenceOwningSymbol},
		StopCondition:    ApplicationEvidenceOwnerResolved,
	}
	return need, need.Validate()
}

func (need ApplicationEvidenceNeed) Validate() error {
	if need.Schema != ApplicationEvidenceNeedSchemaV2 {
		return fmt.Errorf("application evidence need schema must be %q", ApplicationEvidenceNeedSchemaV2)
	}
	if need.ID == "" || need.ID != strings.TrimSpace(need.ID) ||
		len(need.ID) > maxApplicationEvidenceNeedIDBytes || strings.ContainsAny(need.ID, "\r\n") {
		return fmt.Errorf("application evidence need requires one bounded code-owned identity")
	}
	if need.Question == "" || need.Question != strings.TrimSpace(need.Question) ||
		len(need.Question) > maxApplicationEvidenceQuestionBytes {
		return fmt.Errorf("application evidence need %q has an invalid question", need.ID)
	}
	if need.WhyItMatters == "" || need.WhyItMatters != strings.TrimSpace(need.WhyItMatters) ||
		len(need.WhyItMatters) > maxApplicationEvidenceQuestionBytes {
		return fmt.Errorf("application evidence need %q has an invalid reason", need.ID)
	}
	if len(need.SourceClasses) != 1 || need.SourceClasses[0] != ApplicationEvidenceRepository {
		return fmt.Errorf("application evidence need %q has no promoted source-class resolver", need.ID)
	}
	if len(need.RequiredEvidence) != 1 {
		return fmt.Errorf("application evidence need %q requires one registered evidence criterion", need.ID)
	}
	switch need.Kind {
	case ApplicationEvidenceContextFact:
		if need.RequiredEvidence[0] != ApplicationEvidenceDirectlyRelevant ||
			need.StopCondition != ApplicationEvidenceRelevantSelection {
			return fmt.Errorf("application context evidence need %q has an invalid resolver contract", need.ID)
		}
	case ApplicationEvidenceChangeOwner:
		if need.RequiredEvidence[0] != ApplicationEvidenceOwningSymbol ||
			need.StopCondition != ApplicationEvidenceOwnerResolved {
			return fmt.Errorf("application change-owner evidence need %q has an invalid resolver contract", need.ID)
		}
	default:
		return fmt.Errorf("application evidence need %q has unsupported kind %q", need.ID, need.Kind)
	}
	return nil
}

func AppendApplicationContextEvidence(
	context ApplicationContext,
	need ApplicationEvidenceNeed,
	evidence []ApplicationContextEvidence,
) (ApplicationContext, error) {
	if err := context.Validate(); err != nil {
		return ApplicationContext{}, err
	}
	if err := need.Validate(); err != nil {
		return ApplicationContext{}, err
	}
	if context.WorkspaceState != ApplicationWorkspaceExisting {
		return ApplicationContext{}, fmt.Errorf("application repository evidence requires an existing workspace")
	}
	if len(evidence) < 1 || len(context.Facts)+len(evidence) > MaxApplicationContextFacts {
		return ApplicationContext{}, fmt.Errorf("application context cannot admit %d acquired evidence facts", len(evidence))
	}
	seenSources := make(map[string]struct{}, len(context.Facts)+len(evidence))
	for _, fact := range context.Facts {
		seenSources[fact.SourceID] = struct{}{}
	}
	result := context
	result.Facts = append([]ApplicationContextFact(nil), context.Facts...)
	for _, item := range evidence {
		if item.Value == "" || item.Value != strings.TrimSpace(item.Value) ||
			len(item.Value) > MaxApplicationContextFactBytes {
			return ApplicationContext{}, fmt.Errorf("application evidence need %q produced an invalid bounded fact", need.ID)
		}
		if item.SourceID == "" || item.SourceID != strings.TrimSpace(item.SourceID) {
			return ApplicationContext{}, fmt.Errorf("application evidence need %q produced a fact without source identity", need.ID)
		}
		if item.SourceSHA256 != ExactObjectiveContextSHA(item.Value) {
			return ApplicationContext{}, fmt.Errorf("application evidence need %q produced a fact with mismatched source hash", need.ID)
		}
		if _, duplicate := seenSources[item.SourceID]; duplicate {
			return ApplicationContext{}, fmt.Errorf("application context source %q is duplicated", item.SourceID)
		}
		seenSources[item.SourceID] = struct{}{}
		result.Facts = append(result.Facts, ApplicationContextFact{
			ID:           fmt.Sprintf("fact_%03d", len(result.Facts)+1),
			Kind:         ApplicationContextRepositoryFact,
			Authority:    ApplicationContextEvidenceAuthority,
			NeedID:       need.ID,
			Value:        item.Value,
			SourceID:     item.SourceID,
			SourceSHA256: item.SourceSHA256,
		})
	}
	if err := result.Validate(); err != nil {
		return ApplicationContext{}, err
	}
	return result, nil
}
