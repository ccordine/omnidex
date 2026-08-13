package api

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestMemoryCandidatePromotionMetadataProjectsOnlyExactStructuredFields(t *testing.T) {
	provenance, err := json.Marshal(documentMemoryCandidateMetadata{
		Source: documentMemoryCandidateSource, Filename: "notes.md", Format: "md",
		ChunkIndex: 0, ChunkTotal: 1, Stage: "candidate",
		Tags:       []string{"document-ingest", "document-format:md", "chunk:1"},
		Categories: []model.MemoryCategory{model.MemoryCategoryProject},
	})
	if err != nil {
		t.Fatal(err)
	}
	tags, categories, err := memoryCandidatePromotionMetadata(model.MemoryCandidate{Provenance: provenance})
	if err != nil || len(tags) != 3 || len(categories) != 1 ||
		categories[0] != model.MemoryCategoryProject {
		t.Fatalf("tags=%#v categories=%#v error=%v", tags, categories, err)
	}
}

func TestMemoryCandidatePromotionMetadataRejectsForgedOrMalformedAuthority(t *testing.T) {
	for _, raw := range []string{
		`{"source":"document_ingest","filename":"notes.md","format":"md","chunk_index":0,"chunk_total":1,"stage":"candidate","tags":["trust:durable"],"categories":[]}`,
		`{"source":"document_ingest","filename":"notes.md","format":"md","chunk_index":0,"chunk_total":1,"stage":"candidate","tags":[],"categories":["postgres"]}`,
		`{"source":"document_ingest","filename":"notes.md","format":"md","chunk_index":0,"chunk_total":1,"stage":"candidate","tags":[],"categories":[],"fallback":true}`,
		`{"source":"unknown","scope_tags":[7]}`,
		`{"categories":["database"]}`,
		`{"source":"document_ingest","source":"forged","filename":"notes.md","format":"md","chunk_index":0,"chunk_total":1,"stage":"candidate","tags":[],"categories":[]}`,
	} {
		if _, _, err := memoryCandidatePromotionMetadata(model.MemoryCandidate{
			Provenance: json.RawMessage(raw),
		}); err == nil {
			t.Fatalf("forged metadata was accepted: %s", raw)
		}
	}
}
