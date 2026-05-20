package omni

import (
	"fmt"
	"sort"
	"strings"
)

const (
	defaultDocumentChunkChars   = 2400
	defaultDocumentChunkOverlap = 240
)

type DocumentSearchConfig struct {
	ChunkChars   int
	ChunkOverlap int
}

type DocumentChunk struct {
	ID          string
	Index       int
	StartOffset int
	EndOffset   int
	Text        string
}

type DocumentSearchHit struct {
	ChunkID     string
	ChunkIndex  int
	StartOffset int
	EndOffset   int
	Line        int
	Column      int
	Excerpt     string
}

type DocumentWorkerResult struct {
	WorkerID string
	Chunk    DocumentChunk
	Hits     []DocumentSearchHit
}

type DocumentSearchResult struct {
	Query      string
	Chunks     []DocumentChunk
	Workers    []DocumentWorkerResult
	Hits       []DocumentSearchHit
	Found      bool
	ChunkCount int
}

func SearchLargeDocument(document, query string, cfg DocumentSearchConfig) (DocumentSearchResult, error) {
	document = strings.TrimRight(document, "\n")
	query = strings.TrimSpace(query)
	if document == "" {
		return DocumentSearchResult{}, fmt.Errorf("document cannot be empty")
	}
	if query == "" {
		return DocumentSearchResult{}, fmt.Errorf("query cannot be empty")
	}

	cfg = normalizeDocumentSearchConfig(cfg)
	chunks := ChunkDocument(document, cfg)
	result := DocumentSearchResult{
		Query:      query,
		Chunks:     chunks,
		Workers:    make([]DocumentWorkerResult, 0, len(chunks)),
		ChunkCount: len(chunks),
	}

	seenOffsets := map[int]struct{}{}
	for _, chunk := range chunks {
		worker := searchDocumentChunk(document, chunk, query)
		for _, hit := range worker.Hits {
			if _, exists := seenOffsets[hit.StartOffset]; exists {
				continue
			}
			seenOffsets[hit.StartOffset] = struct{}{}
			result.Hits = append(result.Hits, hit)
		}
		result.Workers = append(result.Workers, worker)
	}

	sort.Slice(result.Hits, func(i, j int) bool {
		return result.Hits[i].StartOffset < result.Hits[j].StartOffset
	})
	result.Found = len(result.Hits) > 0
	return result, nil
}

func ChunkDocument(document string, cfg DocumentSearchConfig) []DocumentChunk {
	cfg = normalizeDocumentSearchConfig(cfg)
	if len(document) == 0 {
		return nil
	}

	chunks := make([]DocumentChunk, 0, (len(document)/cfg.ChunkChars)+1)
	step := cfg.ChunkChars - cfg.ChunkOverlap
	if step <= 0 {
		step = cfg.ChunkChars
	}

	for start, index := 0, 0; start < len(document); index++ {
		end := start + cfg.ChunkChars
		if end > len(document) {
			end = len(document)
		}
		chunks = append(chunks, DocumentChunk{
			ID:          fmt.Sprintf("chunk_%04d", index+1),
			Index:       index,
			StartOffset: start,
			EndOffset:   end,
			Text:        document[start:end],
		})
		if end == len(document) {
			break
		}
		start += step
	}
	return chunks
}

func normalizeDocumentSearchConfig(cfg DocumentSearchConfig) DocumentSearchConfig {
	if cfg.ChunkChars <= 0 {
		cfg.ChunkChars = defaultDocumentChunkChars
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 0
	}
	if cfg.ChunkOverlap >= cfg.ChunkChars {
		cfg.ChunkOverlap = cfg.ChunkChars / 10
	}
	return cfg
}

func searchDocumentChunk(document string, chunk DocumentChunk, query string) DocumentWorkerResult {
	worker := DocumentWorkerResult{
		WorkerID: fmt.Sprintf("doc_worker_%04d", chunk.Index+1),
		Chunk:    chunk,
		Hits:     []DocumentSearchHit{},
	}

	searchStart := 0
	for {
		local := strings.Index(chunk.Text[searchStart:], query)
		if local < 0 {
			break
		}
		localStart := searchStart + local
		globalStart := chunk.StartOffset + localStart
		globalEnd := globalStart + len(query)
		line, column := lineColumnAtOffset(document, globalStart)
		worker.Hits = append(worker.Hits, DocumentSearchHit{
			ChunkID:     chunk.ID,
			ChunkIndex:  chunk.Index,
			StartOffset: globalStart,
			EndOffset:   globalEnd,
			Line:        line,
			Column:      column,
			Excerpt:     excerptAround(document, globalStart, globalEnd, 80),
		})
		searchStart = localStart + len(query)
	}
	return worker
}

func lineColumnAtOffset(document string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(document) {
		offset = len(document)
	}
	line := 1
	column := 1
	for i := 0; i < offset; i++ {
		if document[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

func excerptAround(document string, start, end, radius int) string {
	if radius < 0 {
		radius = 0
	}
	excerptStart := start - radius
	if excerptStart < 0 {
		excerptStart = 0
	}
	excerptEnd := end + radius
	if excerptEnd > len(document) {
		excerptEnd = len(document)
	}
	return document[excerptStart:excerptEnd]
}
