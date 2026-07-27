package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/model"
)

func runIngest(c *client.Client, args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	source := fs.String("source", "file", "memory source prefix")
	kind := fs.String("kind", model.MemoryKindReference, "memory kind: episodic|procedural|instruction|preference|reference")
	tags := fs.String("tags", "", "comma-separated tags")
	chunkSize := fs.Int("chunk-size", 1800, "chunk size in characters")
	overlap := fs.Int("overlap", 220, "chunk overlap in characters")
	_ = fs.Parse(args)

	paths := fs.Args()
	if len(paths) == 0 {
		die("ingest requires one or more file paths")
	}

	baseTags := splitTags(*tags)
	totalChunks := 0

	for _, path := range paths {
		parsed, err := ingest.ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: %s: %v\n", path, err)
			continue
		}

		chunks := ingest.ChunkText(parsed.Content, *chunkSize, *overlap)
		if len(chunks) == 0 {
			fmt.Fprintf(os.Stderr, "warn: %s: no ingestible text\n", path)
			continue
		}

		autoTags := ingest.InferTagsFromPath(path, parsed.Format)
		allTags := mergeTags(baseTags, autoTags)
		base := filepath.Base(path)

		storedForFile := 0
		for i, chunk := range chunks {
			chunkSource := fmt.Sprintf("%s:%s#%d", strings.TrimSpace(*source), base, i+1)
			_, err := c.AddMemory(context.Background(), chunkSource, *kind, chunk, allTags)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: %s chunk %d: %v\n", path, i+1, err)
				continue
			}
			storedForFile++
			totalChunks++
		}

		fmt.Printf("ingested %s format=%s chunks=%d\n", path, parsed.Format, storedForFile)
	}

	if totalChunks == 0 {
		die("no chunks were ingested")
	}

	fmt.Printf("ingest complete: %d chunks stored\n", totalChunks)
}
