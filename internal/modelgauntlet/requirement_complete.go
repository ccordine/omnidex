package modelgauntlet

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	maxCompleteRequirementCases   = 100
	maxCompleteRequirementRepeats = 5
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
	var fatalErr error
	decision, err := assemblyline.CompleteRequirementPartition(
		source,
		func(input assemblyline.RequirementPartitionInput) (assemblyline.RequirementPartitionDecision, error) {
			kind := "extract"
			if input.Mode == assemblyline.RequirementSplitFeature {
				kind = "split"
			}
			attempt, callErr := partition(input, kind)
			if callErr != nil {
				fatalErr = callErr
				return assemblyline.RequirementPartitionDecision{}, callErr
			}
			if attempt.Error != "" {
				return assemblyline.RequirementPartitionDecision{}, fmt.Errorf("%s", attempt.Error)
			}
			return attempt.Decision, nil
		},
	)
	if fatalErr != nil {
		return completePartitionAttempt{}, fatalErr
	}
	if err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	return completePartitionAttempt{Decision: decision}, nil
}

func (runner *completeRequirementRunner) nextOperation(kind string) (string, error) {
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
