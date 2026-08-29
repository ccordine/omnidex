package api

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/model"
)

const documentMemoryCandidateSource = "document_ingest"

type documentMemoryCandidateMetadata struct {
	Source     string                 `json:"source"`
	Filename   string                 `json:"filename"`
	Format     string                 `json:"format"`
	ChunkIndex int                    `json:"chunk_index"`
	ChunkTotal int                    `json:"chunk_total"`
	Stage      string                 `json:"stage"`
	Tags       []string               `json:"tags"`
	Categories []model.MemoryCategory `json:"categories"`
}

func memoryCandidatePromotionMetadata(
	candidate model.MemoryCandidate,
) ([]string, []model.MemoryCategory, error) {
	if len(candidate.Provenance) == 0 {
		return nil, nil, nil
	}
	if err := exactjson.ValidateUniqueObject(
		candidate.Provenance, "memory candidate provenance",
	); err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(candidate.Provenance, &envelope); err != nil || envelope == nil {
		return nil, nil, fmt.Errorf("memory candidate provenance must be a JSON object")
	}
	rawSource, exists := envelope["source"]
	if !exists {
		if len(envelope) == 0 {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("memory candidate provenance source is required for structured metadata")
	}
	var source string
	if err := json.Unmarshal(rawSource, &source); err != nil {
		return nil, nil, fmt.Errorf("memory candidate provenance source must be exact text")
	}
	if source != documentMemoryCandidateSource {
		return nil, nil, fmt.Errorf("memory candidate provenance source %q is not registered", source)
	}
	var metadata documentMemoryCandidateMetadata
	if err := exactjson.ValidateObject(
		candidate.Provenance, metadata, "document memory candidate provenance",
	); err != nil {
		return nil, nil, fmt.Errorf("invalid document memory candidate provenance: %w", err)
	}
	if err := json.Unmarshal(candidate.Provenance, &metadata); err != nil {
		return nil, nil, fmt.Errorf("decode document memory candidate provenance: %w", err)
	}
	if metadata.Filename == "" || metadata.Stage != "candidate" ||
		metadata.ChunkTotal < 1 || metadata.ChunkIndex < 0 ||
		metadata.ChunkIndex >= metadata.ChunkTotal {
		return nil, nil, fmt.Errorf("document memory candidate provenance bounds are invalid")
	}
	if _, err := ingest.DocumentFormatTag(metadata.Format); err != nil {
		return nil, nil, err
	}
	if err := model.ValidateMemoryInputTags(metadata.Tags); err != nil {
		return nil, nil, err
	}
	if err := model.ValidateMemoryCategories(metadata.Categories); err != nil {
		return nil, nil, err
	}
	return append([]string(nil), metadata.Tags...),
		append([]model.MemoryCategory(nil), metadata.Categories...), nil
}
