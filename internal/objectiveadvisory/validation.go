package objectiveadvisory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	maxIdentityBytes       = 256
	maxObjectiveBytes      = 4 * 1024
	maxAuthorityBytes      = 5 * 1024
	maxAdviceQuestionBytes = 1024
	maxAdviceListItems     = 16
	maxEvidenceItems       = 12
	maxEvidenceBytes       = 2 * 1024
	maxFailureBytes        = 512
)

func (config Config) Validate() error {
	if err := config.Mode.Validate(); err != nil {
		return err
	}
	if math.IsNaN(config.MinimumRelevance) || math.IsInf(config.MinimumRelevance, 0) ||
		config.MinimumRelevance < -1 || config.MinimumRelevance > 1 {
		return fmt.Errorf("objective advisory minimum relevance must be between -1 and 1")
	}
	if config.Mode == ModeOff {
		if len(config.Sources) > MaxConfiguredSources || config.MaxSelectedCapsules < 0 ||
			config.MaxSelectedCapsules > 1 {
			return fmt.Errorf("off advisory configuration exceeds its inert bounds")
		}
	} else if len(config.Sources) < 1 || len(config.Sources) > MaxConfiguredSources ||
		config.MaxSelectedCapsules != 1 {
		return fmt.Errorf("enabled advisory configuration requires 1..%d sources and exactly one selected capsule", MaxConfiguredSources)
	}
	seen := make(map[string]struct{}, len(config.Sources))
	for index, source := range config.Sources {
		if err := source.validate(); err != nil {
			return fmt.Errorf("objective advisory source %d: %w", index, err)
		}
		if _, duplicate := seen[source.ID]; duplicate {
			return fmt.Errorf("objective advisory source ID %q is duplicated", source.ID)
		}
		seen[source.ID] = struct{}{}
	}
	return nil
}

func (source SourceConfig) validate() error {
	if err := validateLine("source ID", source.ID, maxIdentityBytes); err != nil {
		return err
	}
	if err := validateLine("provider", source.Provider, maxIdentityBytes); err != nil {
		return err
	}
	if err := validateLine("model", source.Model, maxIdentityBytes); err != nil {
		return err
	}
	if math.IsNaN(source.Sampling.Temperature) || math.IsInf(source.Sampling.Temperature, 0) ||
		math.Signbit(source.Sampling.Temperature) || source.Sampling.Temperature > 2 {
		return fmt.Errorf("sampling temperature must be between 0 and 2")
	}
	if source.Sampling.TopP != nil && (math.IsNaN(*source.Sampling.TopP) ||
		math.IsInf(*source.Sampling.TopP, 0) || *source.Sampling.TopP <= 0 || *source.Sampling.TopP > 1) {
		return fmt.Errorf("sampling top_p must be greater than 0 and at most 1")
	}
	if source.Budget.MaxInputBytes < 1 || source.Budget.MaxInputBytes > MaxProjectionBytes+4*1024 ||
		source.Budget.MaxOutputBytes < 1 || source.Budget.MaxOutputBytes > MaxRawTextBytes ||
		source.Budget.MaxOutputTokens < 1 || source.Budget.MaxOutputTokens > 4096 {
		return fmt.Errorf("advisory input/output budget is outside the registered bounds")
	}
	return nil
}

func validateProjectionInput(input ProjectionInput) error {
	if err := validateLine("objective ID", input.ObjectiveID, maxIdentityBytes); err != nil {
		return err
	}
	if input.Generation < 1 {
		return fmt.Errorf("objective advisory generation must be positive")
	}
	if err := validateText("objective", input.Objective, maxObjectiveBytes, false); err != nil {
		return err
	}
	if input.UserAuthorities == nil || len(input.UserAuthorities) < 1 || len(input.UserAuthorities) > maxAdviceListItems {
		return fmt.Errorf("objective advisory requires 1..%d applicable user authorities", maxAdviceListItems)
	}
	seenAuthorities := make(map[string]struct{}, len(input.UserAuthorities))
	for index, authority := range input.UserAuthorities {
		if err := validateLine("user authority ID", authority.ID, maxIdentityBytes); err != nil {
			return fmt.Errorf("user authority %d: %w", index, err)
		}
		if err := validateText("user authority", authority.Content, maxAuthorityBytes, false); err != nil {
			return fmt.Errorf("user authority %d: %w", index, err)
		}
		if _, duplicate := seenAuthorities[authority.ID]; duplicate {
			return fmt.Errorf("user authority ID %q is duplicated", authority.ID)
		}
		seenAuthorities[authority.ID] = struct{}{}
	}
	for label, values := range map[string][]string{
		"constraint": input.Constraints, "decision": input.Decisions,
		"invariant": input.Invariants, "unresolved question": input.UnresolvedQuestions,
	} {
		if values == nil || len(values) > maxAdviceListItems {
			return fmt.Errorf("objective advisory %ss must be an explicit array of at most %d items", label, maxAdviceListItems)
		}
		seen := make(map[string]struct{}, len(values))
		for index, value := range values {
			if err := validateText(label, value, maxAdviceQuestionBytes, false); err != nil {
				return fmt.Errorf("%s %d: %w", label, index, err)
			}
			if _, duplicate := seen[value]; duplicate {
				return fmt.Errorf("objective advisory %s is duplicated", label)
			}
			seen[value] = struct{}{}
		}
	}
	if input.GroundedEvidence == nil || len(input.GroundedEvidence) < 1 || len(input.GroundedEvidence) > maxEvidenceItems {
		return fmt.Errorf("objective advisory requires 1..%d grounded evidence summaries", maxEvidenceItems)
	}
	if err := validateEvidence(input.GroundedEvidence); err != nil {
		return err
	}
	return validateText("useful-advice description", input.UsefulAdvice, maxAdviceQuestionBytes, true)
}

func validateEvidence(evidence []EvidenceSummary) error {
	seen := make(map[string]struct{}, len(evidence))
	for index, item := range evidence {
		if err := validateLine("evidence ID", item.ID, maxIdentityBytes); err != nil {
			return fmt.Errorf("grounded evidence %d: %w", index, err)
		}
		if err := validateText("evidence summary", item.Summary, maxEvidenceBytes, false); err != nil {
			return fmt.Errorf("grounded evidence %s: %w", item.ID, err)
		}
		if !validSHA256(item.SHA256) || item.SHA256 != digest(item.Summary) {
			return fmt.Errorf("grounded evidence %s has invalid exact content identity", item.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("grounded evidence ID %q is duplicated", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func (gap SemanticGap) validateFor(projection Projection) error {
	if gap.ObjectiveID != projection.Input.ObjectiveID || gap.Generation != projection.Input.Generation {
		return fmt.Errorf("advisory semantic gap does not match its grounded objective scope")
	}
	if err := validateText("semantic-gap requirement", gap.Requirement, maxObjectiveBytes, false); err != nil {
		return err
	}
	if err := validateText("semantic-gap candidate", gap.Candidate, maxObjectiveBytes, false); err != nil {
		return err
	}
	if gap.Evidence == nil || len(gap.Evidence) < 1 || len(gap.Evidence) > maxEvidenceItems {
		return fmt.Errorf("advisory semantic gap requires bounded grounded evidence")
	}
	return validateEvidence(gap.Evidence)
}

func (capsule Capsule) ValidateFor(objectiveID string, generation int64) error {
	if capsule.ObjectiveID != objectiveID || capsule.Generation != generation ||
		capsule.Authority != AuthorityNonAuthoritative || capsule.Label != CapsuleLabel {
		return fmt.Errorf("advisory capsule scope, authority, or label is invalid")
	}
	if !validSHA256(capsule.SemanticGapSHA256) {
		return fmt.Errorf("advisory capsule semantic-gap identity is invalid")
	}
	for label, value := range map[string]string{
		"capsule ID": capsule.ID, "source advisory ID": capsule.SourceAdvisoryID,
		"source chunk ID": capsule.SourceChunkID, "provider": capsule.Provider,
		"requested model": capsule.RequestedModel, "effective model": capsule.EffectiveModel,
		"relevance basis": capsule.RelevanceBasis,
	} {
		if err := validateLine(label, value, maxIdentityBytes); err != nil {
			return err
		}
	}
	if err := validateText("capsule content", capsule.Content, MaxCapsuleBytes, true); err != nil {
		return err
	}
	if capsule.ByteCost != len([]byte(capsule.Content)) ||
		capsule.EstimatedTokens != (capsule.ByteCost+3)/4 {
		return fmt.Errorf("advisory capsule byte or token cost is inconsistent")
	}
	return nil
}

func validateLine(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") ||
		len(value) > maximum || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("objective advisory %s must be one exact bounded UTF-8 line", label)
	}
	return nil
}

func validateText(label, value string, maximum int, trimmed bool) error {
	if strings.TrimSpace(value) == "" || len(value) > maximum || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || (trimmed && value != strings.TrimSpace(value)) {
		return fmt.Errorf("objective advisory %s is blank, oversized, noncanonical, or invalid UTF-8", label)
	}
	return nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
