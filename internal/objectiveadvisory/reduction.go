package objectiveadvisory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func chunkArtifact(artifact Artifact) ([]Chunk, error) {
	if artifact.Status != StatusSucceeded || artifact.RawText == "" ||
		artifact.RawTextSHA256 != digest(artifact.RawText) {
		return nil, fmt.Errorf("only a successful exact advisory artifact can be chunked")
	}
	chunks := make([]Chunk, 0, (len(artifact.RawText)+MaxChunkBytes-1)/MaxChunkBytes)
	for cursor := 0; cursor < len(artifact.RawText); {
		end := cursor + MaxChunkBytes
		if end > len(artifact.RawText) {
			end = len(artifact.RawText)
		}
		for end > cursor && !utf8.ValidString(artifact.RawText[cursor:end]) {
			end--
		}
		if end == cursor {
			return nil, fmt.Errorf("advisory chunk boundary cannot preserve valid UTF-8")
		}
		end = preferredChunkEnd(artifact.RawText, cursor, end)
		raw := artifact.RawText[cursor:end]
		trimmed := strings.TrimSpace(raw)
		if trimmed != "" {
			offset := strings.Index(raw, trimmed)
			start := cursor + offset
			finish := start + len(trimmed)
			content := strings.Join(strings.Fields(trimmed), " ")
			if len(content) > MaxCapsuleBytes {
				return nil, fmt.Errorf("minified advisory chunk exceeds %d bytes", MaxCapsuleBytes)
			}
			index := len(chunks)
			sourceHash := digest(artifact.RawText[start:finish])
			chunks = append(chunks, Chunk{
				ID:         digest(fmt.Sprintf("%s\x00%d\x00%d\x00%d\x00%s", artifact.ID, index, start, finish, sourceHash)),
				AdvisoryID: artifact.ID, Index: index, StartByte: start, EndByte: finish,
				SourceTextSHA256: sourceHash, Content: content, ContentSHA256: digest(content),
				Tags: []string{
					"source:objective_advisory", "trigger:" + TriggerPostGroundingObjective,
					"provider:" + artifact.EffectiveProvider,
				},
				ByteCost: len([]byte(content)),
			})
			if len(chunks) > MaxChunksPerArtifact {
				return nil, fmt.Errorf("advisory artifact exceeds the %d-chunk bound", MaxChunksPerArtifact)
			}
		}
		cursor = end
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("advisory artifact contains no minifiable text")
	}
	return chunks, nil
}

func preferredChunkEnd(value string, start, maximum int) int {
	if maximum == len(value) {
		return maximum
	}
	segment := value[start:maximum]
	minimum := len(segment) / 2
	for _, separator := range []string{"\n\n", "\n", ". ", "; ", " "} {
		if index := strings.LastIndex(segment, separator); index >= minimum {
			return start + index + len(separator)
		}
	}
	return maximum
}

type scoredCapsule struct {
	capsule Capsule
	score   float64
}

func reduceRelevantCapsules(
	ctx context.Context,
	embedder Embedder,
	gap SemanticGap,
	artifacts []Artifact,
	chunks []Chunk,
	minimum float64,
) ([]Capsule, int, error) {
	if embedder == nil {
		return nil, 0, fmt.Errorf("objective advisory relevance requires an embedding provider")
	}
	query, err := semanticGapText(gap)
	if err != nil {
		return nil, 0, err
	}
	queryVector, err := embedder.Embedding(ctx, query)
	gapSHA256 := digest(query)
	calls := 1
	if err != nil {
		return nil, calls, fmt.Errorf("embed advisory semantic gap: %w", err)
	}
	if err := validateVector(queryVector); err != nil {
		return nil, calls, fmt.Errorf("advisory semantic-gap embedding: %w", err)
	}
	byID := make(map[string]Artifact, len(artifacts))
	for _, artifact := range artifacts {
		byID[artifact.ID] = artifact
	}
	scored := make([]scoredCapsule, 0, len(chunks))
	for _, chunk := range chunks {
		vector, embedErr := embedder.Embedding(ctx, chunk.Content)
		calls++
		if embedErr != nil {
			return nil, calls, fmt.Errorf("embed advisory chunk %s: %w", chunk.ID, embedErr)
		}
		if err := validateVector(vector); err != nil || len(vector) != len(queryVector) {
			return nil, calls, fmt.Errorf("advisory chunk %s embedding has invalid dimension or values", chunk.ID)
		}
		score, scoreErr := canonicalRelevanceScore(cosine(queryVector, vector))
		if scoreErr != nil {
			return nil, calls, fmt.Errorf("advisory chunk %s relevance is invalid", chunk.ID)
		}
		if score < minimum {
			continue
		}
		artifact, exists := byID[chunk.AdvisoryID]
		if !exists {
			return nil, calls, fmt.Errorf("advisory chunk %s lost its source artifact", chunk.ID)
		}
		relevanceBasis := relevanceBasisPrefix + fmt.Sprintf("%.6f", score)
		capsule := Capsule{
			ID:               capsuleID(chunk.ID, gapSHA256, relevanceBasis),
			SourceAdvisoryID: artifact.ID, SourceChunkID: chunk.ID,
			ObjectiveID: gap.ObjectiveID, Generation: gap.Generation, SemanticGapSHA256: gapSHA256,
			Content:  chunk.Content,
			Provider: artifact.EffectiveProvider, RequestedModel: artifact.RequestedModel,
			EffectiveModel: artifact.EffectiveModel, Authority: AuthorityNonAuthoritative,
			RelevanceBasis: relevanceBasis, Label: CapsuleLabel,
			ByteCost: chunk.ByteCost, EstimatedTokens: (chunk.ByteCost + 3) / 4,
		}
		if err := capsule.ValidateFor(gap.ObjectiveID, gap.Generation); err != nil {
			return nil, calls, err
		}
		scored = append(scored, scoredCapsule{capsule: capsule, score: score})
	}
	sort.Slice(scored, func(left, right int) bool {
		if scored[left].score != scored[right].score {
			return scored[left].score > scored[right].score
		}
		return scored[left].capsule.ID < scored[right].capsule.ID
	})
	result := make([]Capsule, len(scored))
	for index := range scored {
		result[index] = scored[index].capsule
	}
	return result, calls, nil
}

func canonicalRelevanceScore(score float64) (float64, error) {
	if math.IsNaN(score) || math.IsInf(score, 0) || score < -1 || score > 1 {
		return 0, fmt.Errorf("relevance score is non-finite or outside cosine bounds")
	}
	return strconv.ParseFloat(fmt.Sprintf("%.6f", score), 64)
}

func semanticGapText(gap SemanticGap) (string, error) {
	raw, err := json.Marshal(struct {
		Purpose     string            `json:"purpose"`
		Requirement string            `json:"requirement"`
		Candidate   string            `json:"candidate"`
		Evidence    []EvidenceSummary `json:"evidence"`
	}{
		Purpose:     "Find advice relevant to unsupported claims, contradictions, requirement gaps, hidden constraints, edge cases, or verification risks.",
		Requirement: gap.Requirement, Candidate: gap.Candidate,
		Evidence: append([]EvidenceSummary(nil), gap.Evidence...),
	})
	if err != nil {
		return "", fmt.Errorf("encode advisory semantic gap: %w", err)
	}
	return string(raw), nil
}

func validateVector(vector []float64) error {
	if len(vector) == 0 || len(vector) > 16*1024 {
		return fmt.Errorf("embedding vector is empty or oversized")
	}
	norm := float64(0)
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("embedding vector contains a non-finite value")
		}
		norm += value * value
	}
	if norm == 0 {
		return fmt.Errorf("embedding vector has zero magnitude")
	}
	return nil
}

func cosine(left, right []float64) float64 {
	dot, leftNorm, rightNorm := float64(0), float64(0), float64(0)
	for index := range left {
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}
