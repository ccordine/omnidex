package queue

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxMemoryBatchItems        = 512
	maxMemoryBatchContentBytes = 16 << 20
)

var ErrInvalidMemoryWrite = errors.New("invalid memory write")

type MemoryChunkWrite struct {
	Input     model.MemoryInput
	Embedding []float64
}

func (r *Repository) AddMemoryChunk(
	ctx context.Context,
	scope model.MemoryScope,
	source, kind, content string,
	tags []string,
	embedding []float64,
) (model.MemoryChunk, error) {
	parsedSource, err := model.ParseMemorySource(source)
	if err != nil {
		return model.MemoryChunk{}, fmt.Errorf("%w: %v", ErrInvalidMemoryWrite, err)
	}
	parsedKind, err := model.ParseMemoryKind(kind)
	if err != nil {
		return model.MemoryChunk{}, fmt.Errorf("%w: %v", ErrInvalidMemoryWrite, err)
	}
	chunks, err := r.AddMemoryChunks(ctx, []MemoryChunkWrite{{
		Input: model.MemoryInput{
			Scope: scope, Source: parsedSource, Kind: parsedKind, Content: content, Tags: tags,
		},
		Embedding: embedding,
	}})
	if err != nil {
		return model.MemoryChunk{}, err
	}
	return chunks[0], nil
}

func (r *Repository) AddMemoryChunks(
	ctx context.Context,
	writes []MemoryChunkWrite,
) ([]model.MemoryChunk, error) {
	if len(writes) == 0 || len(writes) > maxMemoryBatchItems {
		return nil, fmt.Errorf("%w: memory batch must contain 1..%d writes", ErrInvalidMemoryWrite, maxMemoryBatchItems)
	}
	totalBytes := 0
	for index, write := range writes {
		if err := validateMemoryChunkWrite(write); err != nil {
			return nil, fmt.Errorf("%w: memory batch item %d: %v", ErrInvalidMemoryWrite, index+1, err)
		}
		totalBytes += len(write.Input.Content)
		if totalBytes > maxMemoryBatchContentBytes {
			return nil, fmt.Errorf("%w: memory batch content exceeds %d bytes", ErrInvalidMemoryWrite, maxMemoryBatchContentBytes)
		}
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("memory writes require PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	chunks := make([]model.MemoryChunk, 0, len(writes))
	for _, write := range writes {
		chunk, err := addMemoryChunkTx(ctx, tx, write)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return chunks, nil
}

func validateMemoryChunkWrite(write MemoryChunkWrite) error {
	if err := write.Input.Validate(); err != nil {
		return err
	}
	if len(write.Embedding) != 0 && len(write.Embedding) != model.MemoryEmbeddingDimensions {
		return fmt.Errorf("memory embedding must contain exactly %d values", model.MemoryEmbeddingDimensions)
	}
	for _, value := range write.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("memory embedding values must be finite")
		}
	}
	return nil
}

func addMemoryChunkTx(
	ctx context.Context,
	tx pgx.Tx,
	write MemoryChunkWrite,
) (model.MemoryChunk, error) {
	input := write.Input
	var chunk model.MemoryChunk
	var err error
	if len(write.Embedding) > 0 {
		err = tx.QueryRow(ctx, `
			INSERT INTO memory_chunks (project_id, channel_id, source, kind, content, embedding)
			VALUES ($1, $2, $3, $4, $5, $6::double precision[])
			RETURNING id, project_id, channel_id, source, kind, content, created_at
		`, input.Scope.ProjectID, input.Scope.ChannelID, input.Source, input.Kind,
			input.Content, write.Embedding).Scan(
			&chunk.ID, &chunk.Scope.ProjectID, &chunk.Scope.ChannelID,
			&chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt,
		)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO memory_chunks (project_id, channel_id, source, kind, content)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, project_id, channel_id, source, kind, content, created_at
		`, input.Scope.ProjectID, input.Scope.ChannelID, input.Source, input.Kind,
			input.Content).Scan(
			&chunk.ID, &chunk.Scope.ProjectID, &chunk.Scope.ChannelID,
			&chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt,
		)
	}
	if err != nil {
		return model.MemoryChunk{}, err
	}
	if err := attachMemoryTaxonomyTx(
		ctx, tx, chunk.ID, input.Kind, input.Tags, input.Categories,
	); err != nil {
		return model.MemoryChunk{}, err
	}
	return chunk, nil
}
