package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const maxIngestFileBytes = 32 << 20
const maxIngestFiles = 12

type preparedIngestDocument struct {
	Filename string
	Format   string
	Content  string
	Chunks   []string
	Tags     []string
}

func (s *Server) handleIngestDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository is not configured")
		return
	}
	if err := r.ParseMultipartForm(maxIngestFileBytes * maxIngestFiles); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	stage := r.FormValue("stage")
	if stage != "candidate" && stage != "durable" {
		writeError(w, http.StatusBadRequest, "stage must be exact candidate or durable")
		return
	}
	scope, err := parseMemoryScope(r.FormValue("project_id"), r.FormValue("channel_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind, err := model.ParseMemoryKind(r.FormValue("kind"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	categories, err := parseMultipartMemoryCategories(r.FormValue("categories"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tags, err := parseMultipartMemoryTags(r.FormValue("tags"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 || len(files) > maxIngestFiles {
		writeError(w, http.StatusBadRequest, "upload 1..12 files in the files field")
		return
	}
	chunkSize := parsePositiveInt(r.FormValue("chunk_size"), 1800)
	overlap := parsePositiveInt(r.FormValue("overlap"), 220)
	prepared := make([]preparedIngestDocument, len(files))
	for index, header := range files {
		document, err := prepareIngestDocument(header, chunkSize, overlap, tags)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		prepared[index] = document
	}
	results, err := s.persistPreparedIngest(r, scope, stage, kind, categories, prepared)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"stage": stage, "results": results,
		"message": fmt.Sprintf("ingested %d document(s) as %s", len(results), stage),
	})
}

func prepareIngestDocument(
	header *multipart.FileHeader,
	chunkSize, overlap int,
	extraTags []string,
) (preparedIngestDocument, error) {
	file, err := header.Open()
	if err != nil {
		return preparedIngestDocument{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxIngestFileBytes+1))
	if err != nil {
		return preparedIngestDocument{}, err
	}
	if len(data) > maxIngestFileBytes {
		return preparedIngestDocument{}, fmt.Errorf(
			"%s exceeds %d MiB limit", header.Filename, maxIngestFileBytes>>20,
		)
	}
	parsed, err := ingest.ParseUpload(header.Filename, data)
	if err != nil {
		return preparedIngestDocument{}, fmt.Errorf("%s: %w", header.Filename, err)
	}
	content := strings.TrimSpace(parsed.Content)
	chunks := ingest.ChunkText(content, chunkSize, overlap)
	if content == "" || len(chunks) == 0 {
		return preparedIngestDocument{}, fmt.Errorf("%s: no ingestible text", header.Filename)
	}
	formatTag, err := ingest.DocumentFormatTag(parsed.Format)
	if err != nil {
		return preparedIngestDocument{}, err
	}
	tags := append([]string{"document-ingest", formatTag}, extraTags...)
	if err := model.ValidateMemoryInputTags(tags); err != nil {
		return preparedIngestDocument{}, err
	}
	return preparedIngestDocument{
		Filename: header.Filename, Format: parsed.Format, Content: content,
		Chunks: chunks, Tags: tags,
	}, nil
}

func (s *Server) persistPreparedIngest(
	r *http.Request,
	scope model.MemoryScope,
	stage string,
	kind model.MemoryKind,
	categories []model.MemoryCategory,
	documents []preparedIngestDocument,
) ([]map[string]any, error) {
	totalChunks := 0
	for _, document := range documents {
		totalChunks += len(document.Chunks)
	}
	if totalChunks > maxMemoryBatchItems {
		return nil, fmt.Errorf("document ingest exceeds the %d-chunk batch bound", maxMemoryBatchItems)
	}
	candidates := make([]model.MemoryCandidate, 0, totalChunks)
	writes := make([]queue.MemoryChunkWrite, 0, totalChunks)
	for _, document := range documents {
		for index, chunk := range document.Chunks {
			tags := append(append([]string(nil), document.Tags...), fmt.Sprintf("chunk:%d", index+1))
			if stage == "candidate" {
				provenance, err := documentCandidateProvenance(document, index, stage, tags, categories)
				if err != nil {
					return nil, err
				}
				candidates = append(candidates, model.MemoryCandidate{
					Scope: scope, CandidateKind: kind, Content: chunk, Provenance: provenance,
					Confidence: 0.85, Status: model.MemoryCandidateStatusCandidate,
				})
				continue
			}
			embedding, err := s.requireMemoryEmbedding(r.Context(), chunk)
			if err != nil {
				return nil, fmt.Errorf("embed document chunk %d: %w", index+1, err)
			}
			source, err := validateDocumentMemorySource(document.Filename, index)
			if err != nil {
				return nil, err
			}
			writes = append(writes, queue.MemoryChunkWrite{
				Input: model.MemoryInput{
					Scope: scope, Source: source, Kind: kind, Content: chunk, Tags: tags, Categories: categories,
				},
				Embedding: embedding,
			})
		}
	}
	var candidateIDs, memoryIDs []int64
	var err error
	if stage == "candidate" {
		candidateIDs, err = s.repo.WriteMemoryCandidates(r.Context(), candidates)
	} else {
		var chunks []model.MemoryChunk
		chunks, err = s.repo.AddMemoryChunks(r.Context(), writes)
		for _, chunk := range chunks {
			memoryIDs = append(memoryIDs, chunk.ID)
		}
	}
	if err != nil {
		return nil, err
	}
	return ingestResults(documents, candidateIDs, memoryIDs), nil
}

func documentCandidateProvenance(
	document preparedIngestDocument,
	index int,
	stage string,
	tags []string,
	categories []model.MemoryCategory,
) ([]byte, error) {
	return json.Marshal(map[string]any{
		"source": documentMemoryCandidateSource, "filename": document.Filename,
		"format": document.Format, "chunk_index": index,
		"chunk_total": len(document.Chunks), "stage": stage,
		"tags": tags, "categories": categories,
	})
}

func documentMemorySource(filename string, index int) model.MemorySource {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	slug := strings.NewReplacer(" ", "-", ":", "-", "#", "-").Replace(base)
	return model.MemorySource(fmt.Sprintf("document:%s:%d", slug, index+1))
}
