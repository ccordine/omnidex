package webresearch

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

var objectiveIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func validateObjective(objective Objective) error {
	if !objectiveIDPattern.MatchString(string(objective.ID)) {
		return fmt.Errorf("%w: ID is invalid", ErrInvalidObjective)
	}
	if strings.TrimSpace(objective.Question) == "" || len(objective.Question) > 4_096 {
		return fmt.Errorf("%w: question must contain 1..4096 non-blank exact bytes", ErrInvalidObjective)
	}
	if !utf8.ValidString(objective.Question) || strings.ContainsRune(objective.Question, '\x00') {
		return fmt.Errorf("%w: question must be valid UTF-8 without NUL", ErrInvalidObjective)
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(objective.KnownArtifactPaths)
	if err != nil {
		return fmt.Errorf("%w: known artifact provenance: %v", ErrInvalidObjective, err)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"web research model question", provenance, objective.Question,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObjective, err)
	}
	if _, err := assemblyline.NewConversationObjectiveKindJob(
		assemblyline.ConversationObjectiveKindInput{
			ExactInstruction: objective.Question,
			Context:          objective.Context,
		},
	); err != nil {
		return fmt.Errorf("%w: selected conversation projection: %v", ErrInvalidObjective, err)
	}
	if objective.InitialQuery == "" || objective.InitialQuery != strings.TrimSpace(objective.InitialQuery) ||
		len(objective.InitialQuery) > 4_096 || !utf8.ValidString(objective.InitialQuery) ||
		strings.ContainsRune(objective.InitialQuery, '\x00') {
		return fmt.Errorf("%w: initial query must contain 1..4096 exact trimmed UTF-8 bytes", ErrInvalidObjective)
	}
	if objective.Status != ObjectivePending {
		return fmt.Errorf("%w: objective must begin pending", ErrInvalidObjective)
	}
	return nil
}

func validateConfig(config Config) error {
	if err := validateEvidenceConfig(EvidenceConfig{
		MaxFetchCandidates: config.MaxFetchCandidates, MaxProjectionBytes: config.MaxProjectionBytes,
		MaxRelevantCandidates: config.MaxRelevantCandidates, CandidateSummaryBytes: config.CandidateSummaryBytes,
	}); err != nil {
		return err
	}
	if config.MaxSynthesisParagraphs < 1 || config.MaxSynthesisParagraphs > 4 {
		return fmt.Errorf("%w: synthesis paragraph bound must be 1..4", ErrInvalidConfiguration)
	}
	if config.MaxSynthesisParagraphBytes < 64 || config.MaxSynthesisParagraphBytes > 2_048 {
		return fmt.Errorf("%w: paragraph byte bound must be 64..2048", ErrInvalidConfiguration)
	}
	return nil
}

func validateEvidenceConfig(config EvidenceConfig) error {
	if config.MaxFetchCandidates < 1 || config.MaxFetchCandidates > 32 {
		return fmt.Errorf("%w: fetch candidate bound must be 1..32", ErrInvalidConfiguration)
	}
	if config.MaxProjectionBytes < 256 || config.MaxProjectionBytes > 8_192 {
		return fmt.Errorf("%w: projection bound must be 256..8192 bytes", ErrInvalidConfiguration)
	}
	if config.MaxRelevantCandidates < 1 || config.MaxRelevantCandidates > config.MaxFetchCandidates {
		return fmt.Errorf("%w: relevance bound must fit fetch bound", ErrInvalidConfiguration)
	}
	if config.CandidateSummaryBytes < 64 || config.CandidateSummaryBytes > 2_048 {
		return fmt.Errorf("%w: candidate summary bound must be 64..2048 bytes", ErrInvalidConfiguration)
	}
	if config.MaxFetchCandidates*(config.CandidateSummaryBytes+128) > 8_192 {
		return fmt.Errorf("%w: bounded relevance projection exceeds 8192 bytes", ErrInvalidConfiguration)
	}
	return nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
