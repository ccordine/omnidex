package objectiveadvisory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestChunkingPreservesExactSourceSpansAndBoundsMinifiedText(t *testing.T) {
	raw := strings.Repeat("alpha\t beta gamma delta. ", 70) + "終端"
	artifact := Artifact{
		ID: "artifact-1", ObjectiveID: "objective-17", Generation: 3,
		TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
		ProjectionID: "projection-1", ProjectionSHA256: digest("projection"),
		SourceID: "source-1", Provider: "ollama", RequestedModel: "model-a",
		EffectiveProvider: "ollama", EffectiveModel: "model-a",
		ModelDigest: digest("model-a"), Quantization: "q4", RawText: raw,
		RawTextSHA256: digest(raw), RawBytes: len(raw), CreatedAt: time.Now().UTC(),
		Status: StatusSucceeded, Authority: AuthorityNonAuthoritative,
	}
	chunks, err := chunkArtifact(artifact)
	if err != nil {
		t.Fatalf("chunk exact artifact: %v", err)
	}
	if len(chunks) < 2 || len(chunks) > MaxChunksPerArtifact {
		t.Fatalf("unexpected bounded chunk count %d", len(chunks))
	}
	for index, chunk := range chunks {
		if chunk.Index != index || chunk.StartByte < 0 || chunk.EndByte <= chunk.StartByte ||
			chunk.EndByte > len(raw) || chunk.SourceTextSHA256 != digest(raw[chunk.StartByte:chunk.EndByte]) ||
			chunk.ContentSHA256 != digest(chunk.Content) || chunk.ByteCost != len(chunk.Content) ||
			chunk.ByteCost > MaxChunkBytes || strings.ContainsAny(chunk.Content, "\t\n") {
			t.Fatalf("chunk lost exact span or bounds: %+v", chunk)
		}
		if len(chunk.Tags) != 3 || chunk.Tags[0] != "source:objective_advisory" {
			t.Fatalf("chunk tags are not deterministic structural metadata: %+v", chunk.Tags)
		}
	}
}

func TestRelevanceUsesEmbeddingIdentityRatherThanAdviceTextRouting(t *testing.T) {
	gap := advisoryGap(t)
	query, err := semanticGapText(gap)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := []Artifact{
		{ID: "a", Provider: "p", RequestedModel: "m", EffectiveProvider: "p", EffectiveModel: "m"},
		{ID: "b", Provider: "p", RequestedModel: "m", EffectiveProvider: "p", EffectiveModel: "m"},
	}
	chunks := []Chunk{
		{ID: "c1", AdvisoryID: "a", Content: "arbitrary opaque first text", ByteCost: 27},
		{ID: "c2", AdvisoryID: "b", Content: "arbitrary opaque second text", ByteCost: 28},
	}
	embedder := &mappedEmbedder{vectors: map[string][]float64{
		query: {1, 0}, chunks[0].Content: {0, 1}, chunks[1].Content: {1, 0},
	}}
	capsules, calls, err := reduceRelevantCapsules(
		context.Background(), embedder, gap, artifacts, chunks, 0.5,
	)
	if err != nil {
		t.Fatalf("reduce advisory chunks: %v", err)
	}
	if calls != 3 || len(capsules) != 1 || capsules[0].SourceChunkID != "c2" {
		t.Fatalf("embedding selection did not retain only the relevant opaque ID: %+v", capsules)
	}
}

func TestRelevanceRejectsInvalidEmbeddingAuthority(t *testing.T) {
	gap := advisoryGap(t)
	query, err := semanticGapText(gap)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = reduceRelevantCapsules(context.Background(), &mappedEmbedder{
		vectors: map[string][]float64{query: {1, 0}, "candidate": {1}},
	}, gap, []Artifact{{
		ID: "artifact", Provider: "p", RequestedModel: "m", EffectiveProvider: "p", EffectiveModel: "m",
	}}, []Chunk{{ID: "chunk", AdvisoryID: "artifact", Content: "candidate", ByteCost: 9}}, 0)
	if err == nil {
		t.Fatal("expected embedding dimension mismatch to fail")
	}
}
