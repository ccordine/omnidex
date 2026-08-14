package objectiveadvisory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Runtime struct {
	config   Config
	provider Provider
	embedder Embedder
	clock    Clock
}

func New(config Config, provider Provider, embedder Embedder, clock Clock) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Mode != ModeOff && (provider == nil || embedder == nil) {
		return nil, fmt.Errorf("enabled objective advisory requires provider and embedding boundaries")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Runtime{config: cloneConfig(config), provider: provider, embedder: embedder, clock: clock}, nil
}

func (runtime *Runtime) Run(
	ctx context.Context,
	input ProjectionInput,
	gap SemanticGap,
) (Report, error) {
	if ctx == nil || runtime == nil {
		return Report{}, fmt.Errorf("objective advisory requires context and runtime")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	started := time.Now()
	projection, err := BuildProjection(input)
	if err != nil {
		return Report{}, err
	}
	if err := gap.validateFor(projection); err != nil {
		return Report{}, err
	}
	gapText, err := semanticGapText(gap)
	if err != nil {
		return Report{}, err
	}
	gapSHA256 := digest(gapText)
	report := Report{
		Mode: runtime.config.Mode, TriggerID: TriggerPostGroundingObjective,
		TriggerVersion: TriggerVersionV1, SemanticGapSHA256: gapSHA256, Projection: projection,
		Artifacts: []Artifact{}, Chunks: []Chunk{},
		CandidateCapsules: []Capsule{}, ActiveCapsules: []Capsule{},
		ReductionStatus: StatusNotRun,
	}
	if runtime.config.Mode == ModeOff {
		return runtime.finishReport(report, input, gap, started)
	}
	for _, source := range runtime.config.Sources {
		request := GenerateRequest{
			TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
			Projection: projection, Source: source,
		}
		if _, err := BuildPrompt(request); err != nil {
			return Report{}, err
		}
		report.Metrics.AdvisoryCalls++
		generated, callErr := runtime.provider.Generate(ctx, request)
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		artifact := runtime.ownArtifact(projection, source, generated, callErr)
		report.Artifacts = append(report.Artifacts, artifact)
		report.Metrics.RawBytes += artifact.RawBytes
		report.Metrics.PromptTokens += artifact.PromptTokens
		report.Metrics.OutputTokens += artifact.OutputTokens
		if artifact.Status != StatusSucceeded {
			continue
		}
		chunks, chunkErr := chunkArtifact(artifact)
		if chunkErr != nil {
			return Report{}, chunkErr
		}
		report.Chunks = append(report.Chunks, chunks...)
	}
	report.Metrics.ChunksProduced = len(report.Chunks)
	if len(report.Chunks) == 0 {
		report.ReductionStatus = StatusFailed
		report.ReductionError = "no successful advisory content was eligible for reduction"
		return runtime.finishReport(report, input, gap, started)
	}
	candidates, calls, reduceErr := reduceRelevantCapsules(
		ctx, runtime.embedder, gap, report.Artifacts, report.Chunks,
		runtime.config.MinimumRelevance,
	)
	report.Metrics.EmbeddingCalls = calls
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if reduceErr != nil {
		report.ReductionStatus = StatusFailed
		report.ReductionError = boundedFailure(reduceErr)
		report.Metrics.UnselectedChunks = len(report.Chunks)
		return runtime.finishReport(report, input, gap, started)
	}
	report.ReductionStatus = StatusSucceeded
	report.CandidateCapsules = candidates
	report.Metrics.CandidateCapsules = len(candidates)
	if len(candidates) > 0 {
		report.Metrics.PotentialCapsuleContentBytes = candidates[0].ByteCost
		report.Metrics.PotentialCapsuleContentTokens = candidates[0].EstimatedTokens
		if runtime.config.Mode == ModeActive {
			report.ActiveCapsules = append(report.ActiveCapsules, candidates[0])
			report.Metrics.SelectedCapsules = 1
			report.Metrics.SelectedCapsuleContentBytes = candidates[0].ByteCost
			report.Metrics.SelectedCapsuleContentTokens = candidates[0].EstimatedTokens
		}
	}
	report.Metrics.UnselectedChunks = len(report.Chunks) - report.Metrics.SelectedCapsules
	return runtime.finishReport(report, input, gap, started)
}

func (runtime *Runtime) ownArtifact(
	projection Projection,
	source SourceConfig,
	generation Generation,
	callErr error,
) Artifact {
	created := runtime.clock().UTC()
	artifact := Artifact{
		ObjectiveID: projection.Input.ObjectiveID, Generation: projection.Input.Generation,
		TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
		ProjectionID: projection.ID, ProjectionSHA256: projection.RenderedSHA256,
		SourceID: source.ID, Provider: source.Provider, RequestedModel: source.Model,
		Sampling: source.Sampling, CreatedAt: created, Authority: AuthorityNonAuthoritative,
		Status: StatusFailed,
	}
	if callErr != nil {
		artifact.Failure = boundedFailure(callErr)
		artifact.ID = artifactID(artifact, artifact.Failure)
		return artifact
	}
	artifact.EffectiveProvider = generation.EffectiveProvider
	artifact.EffectiveModel = generation.EffectiveModel
	artifact.ModelDigest = generation.ModelDigest
	artifact.Quantization = generation.Quantization
	artifact.RawBytes = len([]byte(generation.FinalText))
	artifact.RawTextSHA256 = digest(generation.FinalText)
	artifact.PromptTokens = generation.PromptTokens
	artifact.OutputTokens = generation.OutputTokens
	artifact.Duration = generation.Duration
	artifact.FinishReason = generation.FinishReason
	invalid := validateGeneration(generation, source)
	if invalid != nil {
		artifact.Status = StatusInvalid
		artifact.Failure = boundedFailure(invalid)
	} else if generation.FinishReason == "length" {
		artifact.Status = StatusTruncated
		artifact.Failure = "provider stopped at its output length boundary"
	} else if artifact.RawBytes > source.Budget.MaxOutputBytes || artifact.RawBytes > MaxRawTextBytes {
		artifact.Status = StatusInvalid
		artifact.Failure = "provider output exceeded the exact advisory byte budget"
	} else {
		artifact.Status = StatusSucceeded
		artifact.RawText = generation.FinalText
	}
	artifact.ID = artifactID(artifact, artifact.RawTextSHA256+"\x00"+artifact.Failure)
	return artifact
}

func validateGeneration(generation Generation, source SourceConfig) error {
	if err := validateLine("effective provider", generation.EffectiveProvider, maxIdentityBytes); err != nil {
		return err
	}
	if err := validateLine("effective model", generation.EffectiveModel, maxIdentityBytes); err != nil {
		return err
	}
	if !validSHA256(generation.ModelDigest) {
		return fmt.Errorf("objective advisory effective model digest is invalid")
	}
	if err := validateLine("quantization", generation.Quantization, maxIdentityBytes); err != nil {
		return err
	}
	if generation.PromptTokens < 0 || generation.OutputTokens < 0 ||
		generation.OutputTokens > source.Budget.MaxOutputTokens || generation.Duration < 0 ||
		(generation.FinishReason != "stop" && generation.FinishReason != "length") {
		return fmt.Errorf("objective advisory usage or finish reason is invalid")
	}
	if strings.TrimSpace(generation.FinalText) == "" || !utf8.ValidString(generation.FinalText) ||
		strings.ContainsRune(generation.FinalText, '\x00') {
		return fmt.Errorf("objective advisory final text is empty or invalid UTF-8")
	}
	return nil
}

func artifactID(artifact Artifact, terminal string) string {
	topP := "nil"
	if artifact.Sampling.TopP != nil {
		topP = strconv.FormatFloat(*artifact.Sampling.TopP, 'g', -1, 64)
	}
	seed := "nil"
	if artifact.Sampling.Seed != nil {
		seed = strconv.FormatInt(*artifact.Sampling.Seed, 10)
	}
	return digest(strings.Join([]string{
		artifact.ObjectiveID, strconv.FormatInt(artifact.Generation, 10), artifact.TriggerID,
		artifact.TriggerVersion, artifact.ProjectionID, artifact.ProjectionSHA256,
		artifact.SourceID, artifact.Provider, artifact.RequestedModel,
		artifact.EffectiveProvider, artifact.EffectiveModel, artifact.ModelDigest, artifact.Quantization,
		strconv.FormatFloat(artifact.Sampling.Temperature, 'g', -1, 64), topP, seed,
		artifact.RawTextSHA256, strconv.Itoa(artifact.RawBytes), strconv.Itoa(artifact.PromptTokens),
		strconv.Itoa(artifact.OutputTokens), strconv.FormatInt(int64(artifact.Duration), 10),
		artifact.FinishReason, artifact.CreatedAt.Format(time.RFC3339Nano), string(artifact.Status),
		artifact.Failure, artifact.Authority, terminal,
	}, "\x00"))
}

func boundedFailure(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if value == "" {
		value = "objective advisory provider failed without detail"
	}
	if len(value) > maxFailureBytes {
		value = value[:maxFailureBytes]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return value
}

func cloneConfig(config Config) Config {
	config.Sources = append([]SourceConfig(nil), config.Sources...)
	for index := range config.Sources {
		if config.Sources[index].Sampling.TopP != nil {
			value := *config.Sources[index].Sampling.TopP
			config.Sources[index].Sampling.TopP = &value
		}
		if config.Sources[index].Sampling.Seed != nil {
			value := *config.Sources[index].Sampling.Seed
			config.Sources[index].Sampling.Seed = &value
		}
	}
	return config
}
