package api

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func parseMultipartMemoryCategories(value string) ([]model.MemoryCategory, error) {
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	return model.ParseMemoryCategories(parts)
}

func parseMultipartMemoryTags(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	tags := strings.Split(value, ",")
	if err := model.ValidateMemoryInputTags(tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func ingestResults(
	documents []preparedIngestDocument,
	candidateIDs, memoryIDs []int64,
) []map[string]any {
	results := make([]map[string]any, 0, len(documents))
	candidateOffset := 0
	memoryOffset := 0
	for _, document := range documents {
		chunkCount := len(document.Chunks)
		documentCandidateIDs := []int64{}
		if candidateOffset+chunkCount <= len(candidateIDs) {
			documentCandidateIDs = append(documentCandidateIDs,
				candidateIDs[candidateOffset:candidateOffset+chunkCount]...,
			)
			candidateOffset += chunkCount
		}
		documentMemoryIDs := []int64{}
		if memoryOffset+chunkCount <= len(memoryIDs) {
			documentMemoryIDs = append(documentMemoryIDs,
				memoryIDs[memoryOffset:memoryOffset+chunkCount]...,
			)
			memoryOffset += chunkCount
		}
		stage := "candidate"
		if len(documentMemoryIDs) > 0 {
			stage = "durable"
		}
		results = append(results, map[string]any{
			"filename": document.Filename, "format": document.Format,
			"chars": len(document.Content), "chunks": chunkCount, "staged_as": stage,
			"candidate_ids": documentCandidateIDs, "memory_ids": documentMemoryIDs,
		})
	}
	return results
}

func validateDocumentMemorySource(filename string, index int) (model.MemorySource, error) {
	source := documentMemorySource(filename, index)
	if _, err := model.ParseMemorySource(string(source)); err != nil {
		return "", fmt.Errorf("document memory source: %w", err)
	}
	return source, nil
}
