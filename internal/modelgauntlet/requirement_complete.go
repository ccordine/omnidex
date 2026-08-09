package modelgauntlet

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	maxCompleteRequirementCases   = 100
	maxCompleteRequirementRepeats = 5
	maxCompletePartitionCalls     = 128
)

type completePartitionAttempt struct {
	Decision assemblyline.RequirementPartitionDecision
	Error    string
}

type completeRequirementRunner struct {
	ctx        context.Context
	config     CompleteRequirementConfig
	caseID     string
	repetition int
	variant    Variant
	generator  Generator
	report     *CompleteRequirementReport
	sequence   int
}

func RunCompleteRequirementPartition(
	ctx context.Context,
	config CompleteRequirementConfig,
	cases []CompleteRequirementCase,
	generator Generator,
) (CompleteRequirementReport, error) {
	report := CompleteRequirementReport{
		Schema: CompleteRequirementReportSchemaV1, StartedAt: time.Now().UTC(),
		Cases: append([]CompleteRequirementCase(nil), cases...),
		Config: CompleteRequirementConfigEvidence{
			StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
			ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
			Repetitions: config.Repetitions, CasesSHA256: config.CasesSHA256,
			HardwareClass: config.HardwareClass, Backend: config.Backend,
			PromptRenderer:                  CompleteRequirementRendererV3,
			StructuredMaxOutputTokens:       maxStructuredTokens,
			PerSplitAdvisoryMaxOutputTokens: maxDeliberationTokens,
			FinalAdvisoryMaxOutputTokens:    maxFinalRequirementDeliberationTokens,
		},
	}
	if err := validateCompleteRequirementRun(ctx, config, cases, generator); err != nil {
		return report, err
	}
	for repetition := 1; repetition <= config.Repetitions; repetition++ {
		for _, fixture := range cases {
			for _, variant := range completeRequirementVariants() {
				runner := completeRequirementRunner{
					ctx: ctx, config: config, caseID: fixture.ID, repetition: repetition,
					variant: variant, generator: generator, report: &report,
				}
				attempt, err := runner.runVariant(fixture.SourceText)
				if err != nil {
					return report, fmt.Errorf(
						"complete requirement case %q repetition %d variant %q: %w",
						fixture.ID, repetition, variant, err,
					)
				}
				prediction := CompleteRequirementPrediction{
					CaseID: fixture.ID, Repetition: repetition, Variant: variant, Error: attempt.Error,
				}
				if attempt.Error == "" {
					prediction.Valid = true
					prediction.FeatureQuotes = append([]string(nil), attempt.Decision.FeatureQuotes...)
				}
				report.Predictions = append(report.Predictions, prediction)
			}
		}
	}
	report.FinishedAt = time.Now().UTC()
	return report, nil
}

func completeRequirementVariants() []Variant {
	return []Variant{VariantDirect, VariantFinalPassAdvisory, VariantPerSplitAdvisory}
}

func (runner *completeRequirementRunner) runVariant(source string) (completePartitionAttempt, error) {
	switch runner.variant {
	case VariantDirect:
		return runner.runCompletePartition(source, runner.callDirectPartition)
	case VariantPerSplitAdvisory:
		return runner.runCompletePartition(source, runner.callPerSplitPartition)
	case VariantFinalPassAdvisory:
		direct, err := runner.runCompletePartition(source, runner.callDirectPartition)
		if err != nil || direct.Error != "" {
			return direct, err
		}
		return runner.callFinalPass(source, direct.Decision)
	default:
		return completePartitionAttempt{}, fmt.Errorf("variant %q is unsupported", runner.variant)
	}
}

func (runner *completeRequirementRunner) runCompletePartition(
	source string,
	partition func(assemblyline.RequirementPartitionInput, string) (completePartitionAttempt, error),
) (completePartitionAttempt, error) {
	extraction, err := partition(assemblyline.RequirementPartitionInput{
		SourceText: source, Mode: assemblyline.RequirementExtractFeatures,
	}, "extract")
	if err != nil || extraction.Error != "" {
		return extraction, err
	}
	if len(extraction.Decision.FeatureQuotes) == 0 {
		return completePartitionAttempt{Error: "complete requirement extraction returned no feature envelopes"}, nil
	}
	queue := append([]string(nil), extraction.Decision.FeatureQuotes...)
	features := make([]string, 0, len(queue))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result, splitErr := partition(assemblyline.RequirementPartitionInput{
			SourceText: current, Mode: assemblyline.RequirementSplitFeature,
		}, "split")
		if splitErr != nil || result.Error != "" {
			return result, splitErr
		}
		if len(result.Decision.FeatureQuotes) == 1 && result.Decision.FeatureQuotes[0] == current {
			features = append(features, current)
			continue
		}
		for _, child := range result.Decision.FeatureQuotes {
			if len(child) >= len(current) {
				return completePartitionAttempt{Error: fmt.Sprintf(
					"requirement split %q did not make strict progress from %q", child, current,
				)}, nil
			}
			queue = append(queue, child)
		}
	}
	sort.SliceStable(features, func(left, right int) bool {
		return strings.Index(source, features[left]) < strings.Index(source, features[right])
	})
	decision := assemblyline.RequirementPartitionDecision{
		Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: features,
	}
	if err := assemblyline.ValidateCompleteRequirementPartition(source, decision); err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	return completePartitionAttempt{Decision: decision}, nil
}

func (runner *completeRequirementRunner) nextOperation(kind string) (string, error) {
	if runner.sequence >= maxCompletePartitionCalls {
		return "", fmt.Errorf("complete requirement partition exceeded %d bounded operations", maxCompletePartitionCalls)
	}
	runner.sequence++
	return fmt.Sprintf("partition_%03d_%s", runner.sequence, kind), nil
}

func validateCompleteRequirementRun(
	ctx context.Context,
	config CompleteRequirementConfig,
	cases []CompleteRequirementCase,
	generator Generator,
) error {
	if ctx == nil || generator == nil {
		return fmt.Errorf("complete requirement gauntlet requires a context and generator")
	}
	if err := validateAdvisoryConfig(advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}); err != nil {
		return err
	}
	if config.ContextTokens <= maxFinalRequirementDeliberationTokens {
		return fmt.Errorf("context tokens must exceed the %d-token final advisory output budget", maxFinalRequirementDeliberationTokens)
	}
	if config.Repetitions < 1 || config.Repetitions > maxCompleteRequirementRepeats {
		return fmt.Errorf("complete requirement repetitions must be between 1 and %d", maxCompleteRequirementRepeats)
	}
	if err := validateSHA256("complete requirement cases", config.CasesSHA256); err != nil {
		return err
	}
	for label, value := range map[string]string{"hardware class": config.HardwareClass, "backend": config.Backend} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("complete requirement %s must be one trimmed value", label)
		}
	}
	if len(cases) == 0 || len(cases) > maxCompleteRequirementCases {
		return fmt.Errorf("complete requirement cases must contain between 1 and %d cases", maxCompleteRequirementCases)
	}
	seen := make(map[string]struct{}, len(cases))
	for _, fixture := range cases {
		if strings.TrimSpace(fixture.ID) == "" || fixture.ID != strings.TrimSpace(fixture.ID) {
			return fmt.Errorf("complete requirement case requires one trimmed ID")
		}
		if _, exists := seen[fixture.ID]; exists {
			return fmt.Errorf("complete requirement case ID %q is duplicated", fixture.ID)
		}
		seen[fixture.ID] = struct{}{}
		if _, err := assemblyline.NewRequirementPartitionJob(assemblyline.RequirementPartitionInput{
			SourceText: fixture.SourceText, Mode: assemblyline.RequirementExtractFeatures,
		}); err != nil {
			return fmt.Errorf("complete requirement case %q: %w", fixture.ID, err)
		}
	}
	return nil
}

func validateSHA256(label, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 || value != strings.ToLower(value) {
		return fmt.Errorf("%s SHA-256 must be 64 lowercase hexadecimal characters", label)
	}
	return nil
}
