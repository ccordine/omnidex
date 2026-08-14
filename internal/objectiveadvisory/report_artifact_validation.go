package objectiveadvisory

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

func validateReportArtifacts(report Report, config Config) ([]Chunk, error) {
	if config.Mode == ModeOff {
		if len(report.Artifacts) != 0 {
			return nil, fmt.Errorf("off objective advisory report contains artifacts")
		}
		return []Chunk{}, nil
	}
	if len(report.Artifacts) != len(config.Sources) {
		return nil, fmt.Errorf("objective advisory report artifact count escaped configured sources")
	}
	expectedChunks := []Chunk{}
	for index, artifact := range report.Artifacts {
		source := config.Sources[index]
		if err := validateReportArtifact(artifact, report.Projection, source); err != nil {
			return nil, fmt.Errorf("objective advisory artifact %d: %w", index, err)
		}
		if artifact.Status == StatusSucceeded {
			chunks, err := chunkArtifact(artifact)
			if err != nil {
				return nil, err
			}
			expectedChunks = append(expectedChunks, chunks...)
		}
	}
	return expectedChunks, nil
}

func validateReportArtifact(artifact Artifact, projection Projection, source SourceConfig) error {
	if artifact.ObjectiveID != projection.Input.ObjectiveID ||
		artifact.Generation != projection.Input.Generation ||
		artifact.TriggerID != TriggerPostGroundingObjective || artifact.TriggerVersion != TriggerVersionV1 ||
		artifact.ProjectionID != projection.ID || artifact.ProjectionSHA256 != projection.RenderedSHA256 ||
		artifact.SourceID != source.ID || artifact.Provider != source.Provider ||
		artifact.RequestedModel != source.Model || !reflect.DeepEqual(artifact.Sampling, source.Sampling) ||
		artifact.Authority != AuthorityNonAuthoritative {
		return fmt.Errorf("artifact scope, source, or authority provenance is inconsistent")
	}
	if artifact.CreatedAt.IsZero() || artifact.CreatedAt.Location() != time.UTC || artifact.Duration < 0 {
		return fmt.Errorf("artifact creation or duration provenance is invalid")
	}
	if artifact.RawBytes < 0 || artifact.PromptTokens < 0 || artifact.OutputTokens < 0 {
		return fmt.Errorf("artifact byte or token accounting is negative")
	}

	terminal := artifact.RawTextSHA256 + "\x00" + artifact.Failure
	switch artifact.Status {
	case StatusFailed:
		if err := validateText("provider failure", artifact.Failure, maxFailureBytes, true); err != nil ||
			artifact.RawText != "" || artifact.RawTextSHA256 != "" || artifact.RawBytes != 0 ||
			artifact.EffectiveProvider != "" || artifact.EffectiveModel != "" || artifact.ModelDigest != "" ||
			artifact.Quantization != "" || artifact.PromptTokens != 0 || artifact.OutputTokens != 0 ||
			artifact.Duration != 0 || artifact.FinishReason != "" {
			return fmt.Errorf("failed artifact contains generation state or lacks an exact failure")
		}
		terminal = artifact.Failure
	case StatusSucceeded:
		if err := validateSuccessfulArtifact(artifact, source); err != nil {
			return err
		}
	case StatusTruncated:
		if err := validateGeneratedArtifactIdentity(artifact, source); err != nil || artifact.RawText != "" ||
			artifact.FinishReason != "length" || artifact.Failure != "provider stopped at its output length boundary" ||
			artifact.RawBytes < 1 || !validSHA256(artifact.RawTextSHA256) {
			return fmt.Errorf("truncated artifact provenance is invalid")
		}
	case StatusInvalid:
		if artifact.RawText != "" || artifact.Failure == "" || len(artifact.Failure) > maxFailureBytes ||
			artifact.Failure != strings.TrimSpace(artifact.Failure) ||
			(artifact.RawBytes > 0 && !validSHA256(artifact.RawTextSHA256)) {
			return fmt.Errorf("invalid artifact provenance is inconsistent")
		}
	default:
		return fmt.Errorf("artifact status %q is invalid", artifact.Status)
	}
	if artifact.ID != artifactID(artifact, terminal) {
		return fmt.Errorf("artifact ID does not match its terminal provenance")
	}
	return nil
}

func validateSuccessfulArtifact(artifact Artifact, source SourceConfig) error {
	if err := validateGeneratedArtifactIdentity(artifact, source); err != nil {
		return err
	}
	if artifact.FinishReason != "stop" || artifact.Failure != "" ||
		artifact.RawText == "" || artifact.RawBytes != len([]byte(artifact.RawText)) ||
		artifact.RawTextSHA256 != digest(artifact.RawText) || artifact.RawBytes > MaxRawTextBytes ||
		artifact.RawBytes > source.Budget.MaxOutputBytes {
		return fmt.Errorf("successful artifact raw text, status, or accounting is inconsistent")
	}
	return nil
}

func validateGeneratedArtifactIdentity(artifact Artifact, source SourceConfig) error {
	if err := validateLine("effective provider", artifact.EffectiveProvider, maxIdentityBytes); err != nil {
		return err
	}
	if err := validateLine("effective model", artifact.EffectiveModel, maxIdentityBytes); err != nil {
		return err
	}
	if !validSHA256(artifact.ModelDigest) {
		return fmt.Errorf("artifact model digest is invalid")
	}
	if err := validateLine("quantization", artifact.Quantization, maxIdentityBytes); err != nil {
		return err
	}
	if artifact.OutputTokens > source.Budget.MaxOutputTokens {
		return fmt.Errorf("artifact output tokens exceed the configured source")
	}
	return nil
}
