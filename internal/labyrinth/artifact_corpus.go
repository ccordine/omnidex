package labyrinth

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ArtifactCorpusSchemaV1 = "labyrinth.artifact-corpus.v1"
	MaxScaleWorldSize      = 1_000_000
)

// ArtifactCorpusRef is public authority for a lazily generated visible
// artifact collection. It exposes neither its generator seed nor evaluator
// relevance labels.
type ArtifactCorpusRef struct {
	Schema string `json:"schema"`
	Count  int    `json:"count"`
	SHA256 string `json:"sha256"`
}

func (ref ArtifactCorpusRef) Validate() error {
	if ref.Schema != ArtifactCorpusSchemaV1 || ref.Count <= 0 ||
		ref.Count > MaxScaleWorldSize || !validDigest(ref.SHA256) {
		return fmt.Errorf("%w: artifact corpus authority is invalid", ErrGeneration)
	}
	return nil
}

type artifactCorpus struct {
	seed   uint64
	stages []EntityID
	ref    ArtifactCorpusRef
}

func newArtifactCorpus(seed uint64, count int, stages []EntityID) (*artifactCorpus, error) {
	if count <= 0 || count > MaxScaleWorldSize || len(stages) == 0 {
		return nil, fmt.Errorf("%w: artifact corpus coordinates are invalid", ErrGeneration)
	}
	stages = append([]EntityID(nil), stages...)
	for _, stage := range stages {
		if !validSymbol(string(stage)) {
			return nil, fmt.Errorf("%w: artifact corpus stage is invalid", ErrGeneration)
		}
	}
	digest, _, err := digestJSON(struct {
		Schema string     `json:"schema"`
		Count  int        `json:"count"`
		Seed   uint64     `json:"seed"`
		Stages []EntityID `json:"stages"`
	}{ArtifactCorpusSchemaV1, count, seed, stages})
	if err != nil {
		return nil, err
	}
	return &artifactCorpus{
		seed: seed, stages: stages,
		ref: ArtifactCorpusRef{Schema: ArtifactCorpusSchemaV1, Count: count, SHA256: digest},
	}, nil
}

func (corpus *artifactCorpus) clone() *artifactCorpus {
	if corpus == nil {
		return nil
	}
	return &artifactCorpus{seed: corpus.seed, stages: append([]EntityID(nil), corpus.stages...), ref: corpus.ref}
}

func (corpus *artifactCorpus) validate() error {
	if corpus == nil {
		return nil
	}
	rebuilt, err := newArtifactCorpus(corpus.seed, corpus.ref.Count, corpus.stages)
	if err != nil || rebuilt.ref != corpus.ref {
		return fmt.Errorf("%w: private artifact corpus does not match public authority", ErrGeneration)
	}
	return nil
}

func (corpus *artifactCorpus) recordsAt(
	location EntityID,
	includeContent bool,
	limit int,
) ([]ObservedRecord, bool) {
	if corpus == nil || limit <= 0 {
		return nil, corpus != nil && corpus.countAt(location) > 0
	}
	stageIndex := corpus.stageIndex(location)
	if stageIndex < 0 || stageIndex >= corpus.ref.Count {
		return nil, false
	}
	total := corpus.countAt(location)
	count := total
	if count > limit {
		count = limit
	}
	result := make([]ObservedRecord, 0, count)
	for offset, index := 0, stageIndex; offset < count; offset, index = offset+1, index+len(corpus.stages) {
		record := corpus.record(index)
		content := ""
		if includeContent {
			content = record.Content
		}
		result = append(result, ObservedRecord{
			ID: record.ID, Location: record.Location, Content: content,
			ContentSHA256: record.ContentSHA256,
		})
	}
	return result, total > count
}

func (corpus *artifactCorpus) search(query string, limit int) ([]ObservedRecord, int) {
	if corpus == nil || query == "" {
		return []ObservedRecord{}, 0
	}
	index, exists := corpus.queryIndex(query)
	if !exists {
		return []ObservedRecord{}, 0
	}
	record := corpus.record(index)
	if string(record.ID) != query && !strings.Contains(record.Content, query) {
		return []ObservedRecord{}, 0
	}
	if limit <= 0 {
		return []ObservedRecord{}, 1
	}
	return []ObservedRecord{observedRecord(record, true)}, 1
}

func (corpus *artifactCorpus) recordByID(id EntityID) (PublicRecord, bool) {
	if corpus == nil {
		return PublicRecord{}, false
	}
	index, exists := parseCorpusIndex(string(id), "artifact-")
	if !exists || index >= corpus.ref.Count {
		return PublicRecord{}, false
	}
	record := corpus.record(index)
	return record, record.ID == id
}

func (corpus *artifactCorpus) queryIndex(query string) (int, bool) {
	if index, exists := parseCorpusIndex(query, "artifact-"); exists && index < corpus.ref.Count {
		return index, true
	}
	if index, exists := parseCorpusIndex(query, "Catalog entry "); exists && index < corpus.ref.Count {
		return index, true
	}
	return 0, false
}

func parseCorpusIndex(value string, prefix string) (int, bool) {
	if !strings.HasPrefix(value, prefix) || len(value) < len(prefix)+7 {
		return 0, false
	}
	digits := value[len(prefix) : len(prefix)+7]
	index, err := strconv.Atoi(digits)
	if err != nil || fmt.Sprintf("%07d", index) != digits {
		return 0, false
	}
	return index, true
}

func (corpus *artifactCorpus) countAt(location EntityID) int {
	if corpus == nil {
		return 0
	}
	index := corpus.stageIndex(location)
	if index < 0 || index >= corpus.ref.Count {
		return 0
	}
	return 1 + (corpus.ref.Count-1-index)/len(corpus.stages)
}

func (corpus *artifactCorpus) stageIndex(location EntityID) int {
	if corpus == nil {
		return -1
	}
	for index, stage := range corpus.stages {
		if stage == location {
			return index
		}
	}
	return -1
}

func (corpus *artifactCorpus) record(index int) PublicRecord {
	token := textSHA256(fmt.Sprintf("%s\x00%d\x00%d", ArtifactCorpusSchemaV1, corpus.seed, index))
	content := fmt.Sprintf("Catalog entry %07d carries token %s.", index, token[:16])
	record, err := NewPublicRecord(
		EntityID(fmt.Sprintf("artifact-%07d", index)),
		corpus.stages[index%len(corpus.stages)], content,
	)
	if err != nil {
		panic(fmt.Sprintf("construct validated artifact corpus record: %v", err))
	}
	return record
}
