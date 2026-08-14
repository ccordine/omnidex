package objectiveadvisory

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const relevanceBasisPrefix = "cosine_embedding_v1:"

func validateReportCapsules(report Report, config Config) error {
	if report.ReductionStatus != StatusSucceeded &&
		(len(report.CandidateCapsules) != 0 || len(report.ActiveCapsules) != 0) {
		return fmt.Errorf("objective advisory report exposes capsules without a successful reduction")
	}
	chunks := make(map[string]Chunk, len(report.Chunks))
	artifacts := make(map[string]Artifact, len(report.Artifacts))
	for _, artifact := range report.Artifacts {
		artifacts[artifact.ID] = artifact
	}
	for _, chunk := range report.Chunks {
		if _, duplicate := chunks[chunk.ID]; duplicate {
			return fmt.Errorf("objective advisory report contains a duplicate chunk ID")
		}
		chunks[chunk.ID] = chunk
	}

	seenChunks := make(map[string]struct{}, len(report.CandidateCapsules))
	previousScore := math.Inf(1)
	previousID := ""
	for index, capsule := range report.CandidateCapsules {
		score, err := validateReportCapsule(
			capsule, report.Projection, report.SemanticGapSHA256, chunks, artifacts, config.MinimumRelevance,
		)
		if err != nil {
			return fmt.Errorf("objective advisory candidate capsule %d: %w", index, err)
		}
		if _, duplicate := seenChunks[capsule.SourceChunkID]; duplicate {
			return fmt.Errorf("objective advisory candidates duplicate a source chunk")
		}
		seenChunks[capsule.SourceChunkID] = struct{}{}
		if score > previousScore || (score == previousScore && previousID != "" && capsule.ID < previousID) {
			return fmt.Errorf("objective advisory candidates are not in deterministic relevance order")
		}
		previousScore, previousID = score, capsule.ID
	}

	wantActive := []Capsule{}
	if config.Mode == ModeActive && len(report.CandidateCapsules) > 0 {
		wantActive = append(wantActive, report.CandidateCapsules[0])
	}
	if !equalCapsules(report.ActiveCapsules, wantActive) {
		return fmt.Errorf("objective advisory active context is not exactly the first configured candidate")
	}
	return nil
}

func validateReportCapsule(
	capsule Capsule,
	projection Projection,
	semanticGapSHA256 string,
	chunks map[string]Chunk,
	artifacts map[string]Artifact,
	minimum float64,
) (float64, error) {
	if err := capsule.ValidateFor(projection.Input.ObjectiveID, projection.Input.Generation); err != nil {
		return 0, err
	}
	chunk, exists := chunks[capsule.SourceChunkID]
	if !exists || chunk.AdvisoryID != capsule.SourceAdvisoryID {
		return 0, fmt.Errorf("capsule lost its exact source chunk and artifact linkage")
	}
	artifact, exists := artifacts[capsule.SourceAdvisoryID]
	if !exists || artifact.Status != StatusSucceeded || capsule.SemanticGapSHA256 != semanticGapSHA256 ||
		capsule.Content != chunk.Content ||
		capsule.ByteCost != chunk.ByteCost || capsule.Provider != artifact.EffectiveProvider ||
		capsule.RequestedModel != artifact.RequestedModel || capsule.EffectiveModel != artifact.EffectiveModel {
		return 0, fmt.Errorf("capsule content or provider/model provenance differs from its source")
	}
	score, err := parseRelevanceBasis(capsule.RelevanceBasis)
	if err != nil || score < minimum {
		return 0, fmt.Errorf("capsule relevance provenance is invalid or below the configured threshold")
	}
	if capsule.ID != capsuleID(chunk.ID, semanticGapSHA256, capsule.RelevanceBasis) {
		return 0, fmt.Errorf("capsule ID does not match its source and relevance provenance")
	}
	return score, nil
}

func parseRelevanceBasis(value string) (float64, error) {
	if !strings.HasPrefix(value, relevanceBasisPrefix) {
		return 0, fmt.Errorf("unregistered relevance basis")
	}
	raw := strings.TrimPrefix(value, relevanceBasisPrefix)
	score, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(score) || math.IsInf(score, 0) || score < -1 || score > 1 ||
		value != relevanceBasisPrefix+fmt.Sprintf("%.6f", score) {
		return 0, fmt.Errorf("noncanonical relevance score")
	}
	return score, nil
}

func capsuleID(chunkID, semanticGapSHA256, relevanceBasis string) string {
	return digest(strings.Join([]string{chunkID, semanticGapSHA256, relevanceBasis}, "\x00"))
}

func equalCapsules(left, right []Capsule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
