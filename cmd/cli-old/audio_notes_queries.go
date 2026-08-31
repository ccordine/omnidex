package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
)

func runAudioNotesStatus(args []string) {
	fs := flag.NewFlagSet("audio-notes status", flag.ExitOnError)
	root := fs.String("root", defaultAudioNotesRoot, "session root directory")
	sessionID := fs.String("session", "", "session id (defaults to active)")
	_ = fs.Parse(args)

	absRoot, err := filepath.Abs(strings.TrimSpace(*root))
	if err != nil {
		die(err.Error())
	}

	session, err := resolveAudioSession(absRoot, strings.TrimSpace(*sessionID))
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("session=%s status=%s started=%s stopped=%s\n", session.ID, session.Status, session.StartedAt, safeValue(session.StoppedAt, "-"))
	for _, track := range session.Tracks {
		alive := false
		if track.PID > 0 {
			alive = isProcessAlive(track.PID)
		}
		fmt.Printf("track=%s source=%s pid=%d alive=%t audio=%s transcript=%s\n", track.Name, track.Source, track.PID, alive, track.AudioPath, safeValue(track.TranscriptPath, "-"))
	}
	if session.NotesFile != "" {
		fmt.Printf("notes=%s\n", session.NotesFile)
	}
	if session.SegmentsFile != "" {
		fmt.Printf("segments=%s\n", session.SegmentsFile)
	}
}

func runAudioNotesList(args []string) {
	fs := flag.NewFlagSet("audio-notes list", flag.ExitOnError)
	root := fs.String("root", defaultAudioNotesRoot, "session root directory")
	limit := fs.Int("limit", 20, "max sessions")
	_ = fs.Parse(args)

	absRoot, err := filepath.Abs(strings.TrimSpace(*root))
	if err != nil {
		die(err.Error())
	}

	sessions, err := listAudioSessions(absRoot, *limit)
	if err != nil {
		die(err.Error())
	}
	if len(sessions) == 0 {
		fmt.Println("no audio-notes sessions")
		return
	}

	for _, session := range sessions {
		fmt.Printf("%s status=%s started=%s stopped=%s segments=%d\n", session.ID, session.Status, session.StartedAt, safeValue(session.StoppedAt, "-"), session.Transcript.SegmentCount)
	}
}

func runAudioNotesSearch(args []string) {
	fs := flag.NewFlagSet("audio-notes search", flag.ExitOnError)
	root := fs.String("root", defaultAudioNotesRoot, "session root directory")
	sessionID := fs.String("session", "", "session id (defaults to active)")
	limit := fs.Int("limit", 20, "max matches")
	contextWindow := fs.Int("context", 1, "lines before/after each match")
	_ = fs.Parse(args)

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		die("audio-notes search requires a query")
	}

	absRoot, err := filepath.Abs(strings.TrimSpace(*root))
	if err != nil {
		die(err.Error())
	}
	session, err := resolveAudioSession(absRoot, strings.TrimSpace(*sessionID))
	if err != nil {
		die(err.Error())
	}
	if session.SegmentsFile == "" {
		die("session has no transcript segments")
	}
	segments, err := readTranscriptSegments(session.SegmentsFile)
	if err != nil {
		die(err.Error())
	}
	if len(segments) == 0 {
		die("session transcript is empty")
	}

	matches := searchTranscriptSegments(segments, query, *limit)
	if len(matches) == 0 {
		fmt.Printf("no matches for %q in session %s\n", query, session.ID)
		return
	}

	for i, idx := range matches {
		center := segments[idx]
		fmt.Printf("[%d] session=%s source=%s time=%s-%s\n", i+1, session.ID, center.Source, safeValue(center.Start, "?"), safeValue(center.End, "?"))
		beforeStart := idx - *contextWindow
		if beforeStart < 0 {
			beforeStart = 0
		}
		afterEnd := idx + *contextWindow
		if afterEnd >= len(segments) {
			afterEnd = len(segments) - 1
		}
		for j := beforeStart; j <= afterEnd; j++ {
			prefix := "  - "
			if j == idx {
				prefix = "  > "
			}
			entry := segments[j]
			fmt.Printf("%s[%s %s] %s\n", prefix, entry.Source, safeValue(entry.Start, "?"), entry.Text)
		}
	}
}
