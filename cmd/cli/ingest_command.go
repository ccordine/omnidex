package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/model"
)

const maxCLIIngestChunks = 512

type preparedCLIIngest struct {
	Path   string
	Format string
	Chunks int
}

func runIngest(c *client.Client, args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	source := fs.String("source", "", "required memory source prefix")
	projectID := fs.Int64("project-id", 0, "required exact memory project id")
	channelID := fs.String("channel-id", "", "required exact memory channel id")
	kind := fs.String("kind", "", "required memory kind: episodic|procedural|instruction|preference|reference")
	tags := fs.String("tags", "", "comma-separated exact tags")
	categories := fs.String("categories", "", "comma-separated structured categories")
	chunkSize := fs.Int("chunk-size", 1800, "chunk size in characters")
	overlap := fs.Int("overlap", 220, "chunk overlap in characters")
	_ = fs.Parse(args)
	inputs, documents, err := prepareCLIIngest(
		fs.Args(), *projectID, *channelID, *source, *kind, *tags, *categories, *chunkSize, *overlap,
	)
	if err != nil {
		die(err.Error())
	}
	if _, err := c.AddMemories(context.Background(), inputs); err != nil {
		die(err.Error())
	}
	for _, document := range documents {
		fmt.Printf("ingested %s format=%s chunks=%d\n", document.Path, document.Format, document.Chunks)
	}
	fmt.Printf("ingest complete: %d chunks stored\n", len(inputs))
}

func prepareCLIIngest(
	paths []string,
	projectID int64,
	channelID string,
	sourceValue, kindValue, tagsValue, categoryValue string,
	chunkSize, overlap int,
) ([]model.MemoryInput, []preparedCLIIngest, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("ingest requires one or more file paths")
	}
	scope, err := parseCLIMemoryScope(projectID, channelID)
	if err != nil {
		return nil, nil, err
	}
	parsedSource, err := model.ParseMemorySource(sourceValue)
	if err != nil {
		return nil, nil, err
	}
	kind, err := model.ParseMemoryKind(kindValue)
	if err != nil {
		return nil, nil, err
	}
	tags, err := parseCLIMemoryTags(tagsValue)
	if err != nil {
		return nil, nil, err
	}
	categories, err := parseCLIMemoryCategories(categoryValue)
	if err != nil {
		return nil, nil, err
	}
	inputs := make([]model.MemoryInput, 0)
	documents := make([]preparedCLIIngest, 0, len(paths))
	for _, path := range paths {
		parsed, err := ingest.ParseFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		chunks := ingest.ChunkText(parsed.Content, chunkSize, overlap)
		if len(chunks) == 0 {
			return nil, nil, fmt.Errorf("%s: no ingestible text", path)
		}
		formatTag, err := ingest.DocumentFormatTag(parsed.Format)
		if err != nil {
			return nil, nil, err
		}
		documentTags := append([]string{formatTag, "document-ingest"}, tags...)
		if err := model.ValidateMemoryInputTags(documentTags); err != nil {
			return nil, nil, err
		}
		base := filepath.Base(path)
		for index, chunk := range chunks {
			input := model.MemoryInput{
				Scope:  scope,
				Source: model.MemorySource(fmt.Sprintf("%s:%s#%d", parsedSource, base, index+1)),
				Kind:   kind, Content: chunk, Tags: documentTags, Categories: categories,
			}
			if err := input.Validate(); err != nil {
				return nil, nil, fmt.Errorf("%s chunk %d: %w", path, index+1, err)
			}
			inputs = append(inputs, input)
			if len(inputs) > maxCLIIngestChunks {
				return nil, nil, fmt.Errorf("ingest exceeds the %d-chunk batch bound", maxCLIIngestChunks)
			}
		}
		documents = append(documents, preparedCLIIngest{Path: path, Format: parsed.Format, Chunks: len(chunks)})
	}
	return inputs, documents, nil
}
