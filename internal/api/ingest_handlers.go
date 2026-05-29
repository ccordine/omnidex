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
)

const maxIngestFileBytes = 32 << 20 // 32 MiB
const maxIngestFiles = 12

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

	stage := strings.ToLower(strings.TrimSpace(r.FormValue("stage")))
	if stage == "" {
		stage = "candidate"
	}
	if stage != "candidate" && stage != "durable" {
		writeError(w, http.StatusBadRequest, "stage must be candidate or durable")
		return
	}

	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = model.MemoryKindReference
	}

	chunkSize := parsePositiveInt(r.FormValue("chunk_size"), 1800)
	overlap := parsePositiveInt(r.FormValue("overlap"), 220)
	extraTags := splitCommaQuery(r.FormValue("tags"))

	if files := r.MultipartForm.File["files"]; len(files) > 0 {
		results := make([]map[string]any, 0, len(files))
		for _, header := range files {
			if len(results) >= maxIngestFiles {
				break
			}
			result, err := s.ingestUploadedFile(r, header, stage, kind, chunkSize, overlap, extraTags)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			results = append(results, result)
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"stage":   stage,
			"results": results,
			"message": fmt.Sprintf("ingested %d document(s) as %s", len(results), stage),
		})
		return
	}

	writeError(w, http.StatusBadRequest, "upload one or more files in the files field")
}

func (s *Server) ingestUploadedFile(
	r *http.Request,
	header *multipart.FileHeader,
	stage, kind string,
	chunkSize, overlap int,
	extraTags []string,
) (map[string]any, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxIngestFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxIngestFileBytes {
		return nil, fmt.Errorf("%s exceeds %d MiB limit", header.Filename, maxIngestFileBytes>>20)
	}

	parsed, err := ingest.ParseUpload(header.Filename, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", header.Filename, err)
	}
	content := strings.TrimSpace(parsed.Content)
	if content == "" {
		return nil, fmt.Errorf("%s: no extractable text", header.Filename)
	}

	chunks := ingest.ChunkText(content, chunkSize, overlap)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("%s: no ingestible chunks produced", header.Filename)
	}

	baseTags := append([]string{"document-ingest", "pending-review", "trust:pending"}, extraTags...)
	baseTags = append(baseTags, ingest.InferTagsFromPath(header.Filename, parsed.Format)...)
	sourceSlug := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	sourceSlug = strings.ReplaceAll(strings.ToLower(sourceSlug), " ", "-")

	candidateIDs := []int64{}
	memoryIDs := []int64{}

	for index, chunk := range chunks {
		tags := append([]string(nil), baseTags...)
		tags = append(tags, fmt.Sprintf("chunk:%d", index+1))

		provenance, _ := json.Marshal(map[string]any{
			"source":      "document_ingest",
			"filename":    header.Filename,
			"format":      parsed.Format,
			"chunk_index": index,
			"chunk_total": len(chunks),
			"stage":       stage,
			"tags":        tags,
		})

		if stage == "candidate" {
			id, err := s.repo.WriteMemoryCandidate(r.Context(), model.MemoryCandidate{
				CandidateKind: kind,
				Content:       chunk,
				Provenance:    provenance,
				Confidence:    0.85,
				Status:        model.MemoryCandidateStatusCandidate,
			})
			if err != nil {
				return nil, err
			}
			candidateIDs = append(candidateIDs, id)
			continue
		}

		embedding, err := s.llmClient.Embedding(r.Context(), chunk)
		if err != nil {
			embedding = nil
		}
		source := fmt.Sprintf("document:%s:%d", sourceSlug, index+1)
		chunkRow, err := s.repo.AddMemoryChunk(r.Context(), source, kind, chunk, tags, embedding)
		if err != nil {
			return nil, err
		}
		memoryIDs = append(memoryIDs, chunkRow.ID)
	}

	return map[string]any{
		"filename":      header.Filename,
		"format":        parsed.Format,
		"chars":         len(content),
		"chunks":        len(chunks),
		"staged_as":     stage,
		"candidate_ids": candidateIDs,
		"memory_ids":    memoryIDs,
	}, nil
}
