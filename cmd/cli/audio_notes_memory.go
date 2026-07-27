package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
)

func readTranscriptSegments(path string) ([]transcriptSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	var segments []transcriptSegment
	if err := json.Unmarshal(data, &segments); err != nil {
		return nil, err
	}
	return segments, nil
}

func buildNotesDocument(session audioNotesSession, segments []transcriptSegment, warnings []string) string {
	lines := []string{
		"# Call Notes Transcript",
		"",
		"session=" + session.ID,
		"started_at=" + session.StartedAt,
		"stopped_at=" + safeValue(session.StoppedAt, "unknown"),
	}
	if session.Capture.MicEnabled {
		lines = append(lines, "mic_source="+safeValue(session.Capture.MicSource, "unknown"))
	}
	if session.Capture.SpeakerEnabled {
		lines = append(lines, "speaker_source="+safeValue(session.Capture.SpeakerSource, "unknown"))
	}
	if len(warnings) > 0 {
		lines = append(lines, "warnings="+strings.Join(warnings, " | "))
	}
	lines = append(lines, "", "## Transcript", "")
	for _, segment := range segments {
		lines = append(lines, fmt.Sprintf("[%s %s-%s] %s", segment.Source, safeValue(segment.Start, "?"), safeValue(segment.End, "?"), segment.Text))
	}
	return strings.Join(lines, "\n")
}

func storeAudioNotesMemory(c *client.Client, session audioNotesSession, notes, sourcePrefix, kind string, extraTags []string, chunkSize, overlap, maxChunks int) (int, []string, error) {
	if c == nil {
		return 0, nil, errors.New("client is required for memory storage")
	}
	if strings.TrimSpace(notes) == "" {
		return 0, nil, errors.New("notes are empty")
	}
	chunks := ingest.ChunkText(notes, chunkSize, overlap)
	if len(chunks) == 0 {
		return 0, nil, errors.New("notes produced no chunks")
	}
	if maxChunks > 0 && len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
	}

	tags := mergeTags(extraTags, []string{"audio-notes", "transcript", "call-notes", "session-" + session.ID})
	prefix := strings.TrimSpace(sourcePrefix)
	if prefix == "" {
		prefix = "audio-notes"
	}
	slug := sanitizeMemorySourceToken(session.ID)
	if slug == "" {
		slug = fmt.Sprintf("session-%d", time.Now().Unix())
	}

	stored := 0
	for i, chunk := range chunks {
		source := fmt.Sprintf("%s:%s#%03d", prefix, slug, i+1)
		if _, err := c.AddMemory(context.Background(), source, kind, chunk, tags); err != nil {
			return stored, tags, err
		}
		stored++
	}
	return stored, tags, nil
}

func searchTranscriptSegments(segments []transcriptSegment, query string, limit int) []int {
	tokens := strings.Fields(normalizeForMatch(query))
	if len(tokens) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}
	indices := make([]int, 0, limit)
	for idx, segment := range segments {
		normalized := normalizeForMatch(segment.Text)
		match := true
		for _, token := range tokens {
			if !strings.Contains(normalized, token) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		indices = append(indices, idx)
		if len(indices) >= limit {
			break
		}
	}
	return indices
}

func discoverAudioNotesSessions(root string) ([]string, error) {
	paths := make([]string, 0, 32)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), "session.json") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
