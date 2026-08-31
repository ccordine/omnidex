package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func validateAudioNotesMemoryOptions(
	enabled bool,
	projectID int64,
	channelID string,
	source, kind, tags string,
) (model.MemoryScope, []string, error) {
	if !enabled {
		return model.MemoryScope{}, nil, nil
	}
	scope, err := parseCLIMemoryScope(projectID, channelID)
	if err != nil {
		return model.MemoryScope{}, nil, err
	}
	if _, err := model.ParseMemorySource(source); err != nil {
		return model.MemoryScope{}, nil, err
	}
	if _, err := model.ParseMemoryKind(kind); err != nil {
		return model.MemoryScope{}, nil, err
	}
	parsedTags, err := parseCLIMemoryTags(tags)
	return scope, parsedTags, err
}

func storeAudioNotesSessionMemory(
	c *client.Client,
	session audioNotesSession,
	scope model.MemoryScope,
	source, kind string,
	tags []string,
	chunkSize, overlap, maxChunks int,
	storedAt time.Time,
) (audioMemoryState, error) {
	if session.NotesFile == "" {
		return audioMemoryState{}, errors.New("memory ingest requested but transcription produced no notes")
	}
	notesData, err := os.ReadFile(session.NotesFile)
	if err != nil {
		return audioMemoryState{}, fmt.Errorf("read notes for memory ingest: %w", err)
	}
	stored, tagsUsed, err := storeAudioNotesMemory(
		c, session, string(notesData), scope, source, kind, tags, chunkSize, overlap, maxChunks,
	)
	if err != nil {
		return audioMemoryState{}, err
	}
	return audioMemoryState{
		StoredChunks: stored, SourcePrefix: source, Tags: tagsUsed,
		StoredAt: storedAt.Format(time.RFC3339),
	}, nil
}

func printAudioNotesStopResult(session audioNotesSession, warnings []string) {
	fmt.Printf("audio-notes stopped session=%s status=%s\n", session.ID, session.Status)
	if session.SegmentsFile != "" {
		fmt.Printf("segments=%s count=%d\n", session.SegmentsFile, session.Transcript.SegmentCount)
	}
	if session.NotesFile != "" {
		fmt.Printf("notes=%s\n", session.NotesFile)
	}
	if session.Memory.StoredChunks > 0 {
		fmt.Printf("memory_stored=%d source=%s\n", session.Memory.StoredChunks, session.Memory.SourcePrefix)
	}
	if len(warnings) > 0 {
		fmt.Println("warnings:")
		for _, warning := range warnings {
			fmt.Println("- " + warning)
		}
	}
}
