package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/media"
	"github.com/gryph/omnidex/internal/model"
)

const defaultAudioNotesRoot = ".omni/audio-notes"
const audioCaptureSampleRate = 16000
const audioCaptureChannels = 1
const audioStopGracePeriod = 8 * time.Second

type audioNotesSession struct {
	ID            string               `json:"id"`
	RootDir       string               `json:"root_dir"`
	SessionDir    string               `json:"session_dir"`
	CreatedAt     string               `json:"created_at"`
	StartedAt     string               `json:"started_at"`
	StoppedAt     string               `json:"stopped_at,omitempty"`
	Status        string               `json:"status"`
	Capture       audioCaptureSettings `json:"capture"`
	Tracks        []audioTrackState    `json:"tracks"`
	Transcript    audioTranscriptState `json:"transcript"`
	Memory        audioMemoryState     `json:"memory"`
	NotesFile     string               `json:"notes_file,omitempty"`
	SegmentsFile  string               `json:"segments_file,omitempty"`
	LastUpdatedAt string               `json:"last_updated_at"`
}

type audioCaptureSettings struct {
	Backend        string `json:"backend"`
	MicEnabled     bool   `json:"mic_enabled"`
	SpeakerEnabled bool   `json:"speaker_enabled"`
	MicSource      string `json:"mic_source,omitempty"`
	SpeakerSource  string `json:"speaker_source,omitempty"`
}

type audioTrackState struct {
	Name           string `json:"name"`
	Source         string `json:"source"`
	AudioPath      string `json:"audio_path"`
	PID            int    `json:"pid,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	StoppedAt      string `json:"stopped_at,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
}

type audioTranscriptState struct {
	Enabled      bool   `json:"enabled"`
	Engine       string `json:"engine,omitempty"`
	Model        string `json:"model,omitempty"`
	GeneratedAt  string `json:"generated_at,omitempty"`
	Status       string `json:"status,omitempty"`
	Error        string `json:"error,omitempty"`
	SegmentCount int    `json:"segment_count,omitempty"`
}

type audioMemoryState struct {
	StoredChunks int      `json:"stored_chunks,omitempty"`
	SourcePrefix string   `json:"source_prefix,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	StoredAt     string   `json:"stored_at,omitempty"`
}

type audioActiveSession struct {
	SessionID string `json:"session_id"`
	UpdatedAt string `json:"updated_at"`
}

type transcriptSegment struct {
	Source string `json:"source"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Text   string `json:"text"`
}

type audioTranscriberSpec struct {
	Name  string
	Model string
}

func runAudioNotes(c *client.Client, args []string) {
	sub := "help"
	rest := args
	if len(args) > 0 {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
		rest = args[1:]
	}

	switch sub {
	case "help", "":
		printAudioNotesHelp()
	case "doctor":
		runAudioNotesDoctor(rest)
	case "start":
		runAudioNotesStart(rest)
	case "stop":
		runAudioNotesStop(c, rest)
	case "status":
		runAudioNotesStatus(rest)
	case "list":
		runAudioNotesList(rest)
	case "search":
		runAudioNotesSearch(rest)
	default:
		die("unknown audio-notes command. use `omni audio-notes help`")
	}
}

func printAudioNotesHelp() {
	fmt.Println("audio-notes commands:")
	fmt.Println("  audio-notes doctor")
	fmt.Println("  audio-notes start [--root dir] [--session id] [--mic] [--speaker] [--mic-source name] [--speaker-source name]")
	fmt.Println("  audio-notes stop [--root dir] [--session id] [--transcribe] [--transcriber auto|whisper|whisper-cli|none] [--model value] [--language en] [--store-memory] [--source audio-notes] [--kind reference] [--tags a,b] [--chunk-size N] [--overlap N] [--max-chunks N]")
	fmt.Println("  audio-notes status [--root dir] [--session id]")
	fmt.Println("  audio-notes list [--root dir] [--limit N]")
	fmt.Println("  audio-notes search [--root dir] [--session id] [--limit N] [--context N] <query>")
	fmt.Println("")
	fmt.Println("notes:")
	fmt.Println("  - captures mic/speaker with ffmpeg + PulseAudio sources")
	fmt.Println("  - stop performs optional transcription and memory ingest")
	fmt.Println("  - invasive actions require saved permissions (see `omni permissions ...`)")
}

func runAudioNotesDoctor(args []string) {
	fs := flag.NewFlagSet("audio-notes doctor", flag.ExitOnError)
	root := fs.String("root", defaultAudioNotesRoot, "session root directory")
	_ = fs.Parse(args)

	lines := []string{"Audio notes doctor:"}
	lines = append(lines, "root="+strings.TrimSpace(*root))

	if _, err := exec.LookPath("ffmpeg"); err == nil {
		lines = append(lines, "ffmpeg=available")
	} else {
		lines = append(lines, "ffmpeg=missing (required for capture)")
	}

	if _, err := exec.LookPath("pactl"); err == nil {
		lines = append(lines, "pactl=available")
		if info, err := runAudioCommand("pactl", "info"); err == nil {
			mic := parsePactlInfoValue(info, "Default Source")
			sink := parsePactlInfoValue(info, "Default Sink")
			lines = append(lines, "default_mic_source="+safeValue(mic, "unknown"))
			if sink != "" {
				lines = append(lines, "default_speaker_source="+sink+".monitor")
			}
		}
		if sources, err := runAudioCommand("pactl", "list", "short", "sources"); err == nil {
			sourceLines := strings.Split(strings.TrimSpace(sources), "\n")
			if len(sourceLines) > 0 && strings.TrimSpace(sourceLines[0]) != "" {
				lines = append(lines, "sources_detected="+strconv.Itoa(len(sourceLines)))
			}
		}
	} else {
		lines = append(lines, "pactl=missing (required for Pulse source discovery)")
	}

	transcriber, _, detail := detectTranscriber("auto", "", "en")
	lines = append(lines, "transcriber_auto="+safeValue(transcriber.Name, "none"))
	if strings.TrimSpace(detail) != "" {
		lines = append(lines, "transcriber_detail="+detail)
	}

	pm := getPermissionManager()
	path, entries, err := pm.List()
	if err != nil {
		lines = append(lines, "permissions_error="+err.Error())
	} else {
		lines = append(lines, "permissions_store="+path)
		lines = append(lines, "saved_permissions="+strconv.Itoa(len(entries)))
	}

	fmt.Println(strings.Join(lines, "\n"))
}

func runAudioNotesStart(args []string) {
	fs := flag.NewFlagSet("audio-notes start", flag.ExitOnError)
	root := fs.String("root", defaultAudioNotesRoot, "session root directory")
	sessionID := fs.String("session", "", "optional session id")
	mic := fs.Bool("mic", true, "capture microphone input")
	speaker := fs.Bool("speaker", true, "capture speaker/monitor output")
	micSource := fs.String("mic-source", "", "PulseAudio source name for mic")
	speakerSource := fs.String("speaker-source", "", "PulseAudio monitor source name for speaker")
	_ = fs.Parse(args)

	if !*mic && !*speaker {
		die("at least one of --mic or --speaker must be enabled")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		die("ffmpeg not found (install ffmpeg to capture audio)")
	}
	if _, err := exec.LookPath("pactl"); err != nil {
		die("pactl not found (install pulseaudio-utils / pipewire-pulse tools)")
	}

	if *mic {
		if err := ensureLocalPermission(permissionKeyAudioMic, "Allow recording microphone audio for long-running notes."); err != nil {
			die(err.Error())
		}
	}
	if *speaker {
		if err := ensureLocalPermission(permissionKeyAudioSpeaker, "Allow recording speaker/monitor audio for long-running notes."); err != nil {
			die(err.Error())
		}
	}

	absRoot, err := filepath.Abs(strings.TrimSpace(*root))
	if err != nil {
		die(err.Error())
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		die(err.Error())
	}

	if active, err := loadActiveAudioSession(absRoot); err == nil && strings.TrimSpace(active.SessionID) != "" {
		if existing, err := loadAudioSession(absRoot, active.SessionID); err == nil && strings.EqualFold(existing.Status, "running") {
			die(fmt.Sprintf("audio-notes session %q is already running. Stop it first with `omni audio-notes stop --session %s`", existing.ID, existing.ID))
		}
	}

	resolvedMic, resolvedSpeaker, err := resolveAudioSources(*mic, *speaker, *micSource, *speakerSource)
	if err != nil {
		die(err.Error())
	}

	now := time.Now().UTC()
	id := strings.TrimSpace(*sessionID)
	if id == "" {
		id = "session-" + now.Format("20060102-150405")
	}
	id = sanitizeMemorySourceToken(id)
	if id == "" {
		id = "session-" + strconv.FormatInt(now.Unix(), 10)
	}

	sessionDir := filepath.Join(absRoot, id)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		die(err.Error())
	}

	session := audioNotesSession{
		ID:         id,
		RootDir:    absRoot,
		SessionDir: sessionDir,
		CreatedAt:  now.Format(time.RFC3339),
		StartedAt:  now.Format(time.RFC3339),
		Status:     "running",
		Capture: audioCaptureSettings{
			Backend:        "pulse+ffmpeg",
			MicEnabled:     *mic,
			SpeakerEnabled: *speaker,
			MicSource:      resolvedMic,
			SpeakerSource:  resolvedSpeaker,
		},
		Tracks:        []audioTrackState{},
		LastUpdatedAt: now.Format(time.RFC3339),
	}

	if *mic {
		path := filepath.Join(sessionDir, "mic.wav")
		pid, err := startAudioCaptureProcess(resolvedMic, path)
		if err != nil {
			die("failed starting mic capture: " + err.Error())
		}
		session.Tracks = append(session.Tracks, audioTrackState{
			Name:      "mic",
			Source:    resolvedMic,
			AudioPath: path,
			PID:       pid,
			StartedAt: now.Format(time.RFC3339),
		})
	}
	if *speaker {
		path := filepath.Join(sessionDir, "speaker.wav")
		pid, err := startAudioCaptureProcess(resolvedSpeaker, path)
		if err != nil {
			die("failed starting speaker capture: " + err.Error())
		}
		session.Tracks = append(session.Tracks, audioTrackState{
			Name:      "speaker",
			Source:    resolvedSpeaker,
			AudioPath: path,
			PID:       pid,
			StartedAt: now.Format(time.RFC3339),
		})
	}

	if err := saveAudioSession(session); err != nil {
		die(err.Error())
	}
	if err := saveActiveAudioSession(absRoot, audioActiveSession{SessionID: id, UpdatedAt: now.Format(time.RFC3339)}); err != nil {
		fmt.Fprintf(os.Stderr, "warn: failed writing active session pointer: %v\n", err)
	}

	fmt.Printf("audio-notes started session=%s root=%s\n", id, absRoot)
	for _, track := range session.Tracks {
		fmt.Printf("track=%s source=%s pid=%d file=%s\n", track.Name, track.Source, track.PID, track.AudioPath)
	}
	fmt.Println("stop with: omni audio-notes stop --session " + id)
}

func runAudioNotesStop(c *client.Client, args []string) {
	fs := flag.NewFlagSet("audio-notes stop", flag.ExitOnError)
	root := fs.String("root", defaultAudioNotesRoot, "session root directory")
	sessionID := fs.String("session", "", "session id (defaults to active)")
	transcribe := fs.Bool("transcribe", true, "transcribe captured audio after stopping")
	transcriber := fs.String("transcriber", "auto", "transcriber backend: auto|whisper|whisper-cli|none")
	modelValue := fs.String("model", "", "transcriber model (whisper: model name, whisper-cli: model path)")
	language := fs.String("language", "en", "transcription language hint")
	storeMemory := fs.Bool("store-memory", true, "store transcript notes into long-term memory")
	sourcePrefix := fs.String("source", "audio-notes", "memory source prefix")
	kind := fs.String("kind", model.MemoryKindReference, "memory kind")
	tags := fs.String("tags", "", "extra tags")
	chunkSize := fs.Int("chunk-size", 1800, "memory chunk size")
	overlap := fs.Int("overlap", 220, "memory chunk overlap")
	maxChunks := fs.Int("max-chunks", 40, "max memory chunks to store")
	_ = fs.Parse(args)

	absRoot, err := filepath.Abs(strings.TrimSpace(*root))
	if err != nil {
		die(err.Error())
	}

	session, err := resolveAudioSession(absRoot, strings.TrimSpace(*sessionID))
	if err != nil {
		die(err.Error())
	}

	now := time.Now().UTC()
	for i := range session.Tracks {
		track := &session.Tracks[i]
		if track.PID <= 0 {
			continue
		}
		_ = stopAudioProcess(track.PID, audioStopGracePeriod)
		track.StoppedAt = now.Format(time.RFC3339)
		track.PID = 0
	}
	if strings.EqualFold(session.Status, "running") {
		session.Status = "stopped"
		session.StoppedAt = now.Format(time.RFC3339)
	}
	session.LastUpdatedAt = now.Format(time.RFC3339)

	segments := make([]transcriptSegment, 0, 128)
	transcriptionWarnings := make([]string, 0, 4)
	engineUsed := "none"
	modelUsed := ""

	if *transcribe {
		if err := ensureLocalPermission(permissionKeyAudioTranscribe, "Allow converting captured call audio into transcripts and notes."); err != nil {
			transcriptionWarnings = append(transcriptionWarnings, err.Error())
		} else {
			spec, supported, detail := detectTranscriber(*transcriber, *modelValue, *language)
			if !supported || spec.Name == "none" {
				if detail == "" {
					detail = "no available transcription backend"
				}
				transcriptionWarnings = append(transcriptionWarnings, detail)
			} else {
				engineUsed = spec.Name
				modelUsed = spec.Model
				for i := range session.Tracks {
					track := &session.Tracks[i]
					if strings.TrimSpace(track.AudioPath) == "" {
						continue
					}
					transcriptPath, err := transcribeAudioTrack(spec, track.Name, track.AudioPath, session.SessionDir, *language)
					if err != nil {
						transcriptionWarnings = append(transcriptionWarnings, track.Name+": "+err.Error())
						continue
					}
					track.TranscriptPath = transcriptPath
					trackSegments, err := parseTranscriptSegments(track.Name, transcriptPath)
					if err != nil {
						transcriptionWarnings = append(transcriptionWarnings, track.Name+" parse: "+err.Error())
						continue
					}
					segments = append(segments, trackSegments...)
				}
			}
		}
	}

	sortTranscriptSegments(segments)
	if len(segments) > 0 {
		session.SegmentsFile = filepath.Join(session.SessionDir, "segments.json")
		session.NotesFile = filepath.Join(session.SessionDir, "notes.md")
		if err := writeTranscriptSegments(session.SegmentsFile, segments); err != nil {
			transcriptionWarnings = append(transcriptionWarnings, "segments write: "+err.Error())
		}
		notes := buildNotesDocument(session, segments, transcriptionWarnings)
		if err := os.WriteFile(session.NotesFile, []byte(notes+"\n"), 0o644); err != nil {
			transcriptionWarnings = append(transcriptionWarnings, "notes write: "+err.Error())
		}

		session.Transcript = audioTranscriptState{
			Enabled:      true,
			Engine:       engineUsed,
			Model:        modelUsed,
			GeneratedAt:  now.Format(time.RFC3339),
			Status:       "ready",
			SegmentCount: len(segments),
		}
	} else {
		status := "skipped"
		if *transcribe {
			status = "empty"
		}
		session.Transcript = audioTranscriptState{
			Enabled:     *transcribe,
			Engine:      engineUsed,
			Model:       modelUsed,
			GeneratedAt: now.Format(time.RFC3339),
			Status:      status,
			Error:       strings.Join(transcriptionWarnings, " | "),
		}
	}

	if *storeMemory && strings.TrimSpace(session.NotesFile) != "" {
		notesData, err := os.ReadFile(session.NotesFile)
		if err != nil {
			transcriptionWarnings = append(transcriptionWarnings, "memory read notes: "+err.Error())
		} else {
			stored, tagsUsed, err := storeAudioNotesMemory(c, session, string(notesData), strings.TrimSpace(*sourcePrefix), *kind, splitTags(*tags), *chunkSize, *overlap, *maxChunks)
			if err != nil {
				transcriptionWarnings = append(transcriptionWarnings, "memory store: "+err.Error())
			} else {
				session.Memory = audioMemoryState{
					StoredChunks: stored,
					SourcePrefix: strings.TrimSpace(*sourcePrefix),
					Tags:         tagsUsed,
					StoredAt:     now.Format(time.RFC3339),
				}
			}
		}
	}

	session.LastUpdatedAt = now.Format(time.RFC3339)
	if len(transcriptionWarnings) > 0 {
		if session.Transcript.Error == "" {
			session.Transcript.Error = strings.Join(transcriptionWarnings, " | ")
		}
	}

	if err := saveAudioSession(session); err != nil {
		die(err.Error())
	}
	clearActiveAudioSession(absRoot, session.ID)

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
	if len(transcriptionWarnings) > 0 {
		fmt.Println("warnings:")
		for _, warning := range transcriptionWarnings {
			fmt.Println("- " + warning)
		}
	}
}

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

func resolveAudioSession(root, explicitID string) (audioNotesSession, error) {
	id := strings.TrimSpace(explicitID)
	if id == "" {
		active, err := loadActiveAudioSession(root)
		if err != nil {
			return audioNotesSession{}, err
		}
		id = strings.TrimSpace(active.SessionID)
	}
	if id == "" {
		return audioNotesSession{}, errors.New("no session id provided and no active audio-notes session found")
	}
	return loadAudioSession(root, id)
}

func listAudioSessions(root string, limit int) ([]audioNotesSession, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]audioNotesSession, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := loadAudioSession(root, entry.Name())
		if err != nil {
			continue
		}
		out = append(out, session)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt > out[j].StartedAt
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func loadAudioSession(root, sessionID string) (audioNotesSession, error) {
	path := filepath.Join(root, sessionID, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return audioNotesSession{}, err
	}

	var session audioNotesSession
	if err := json.Unmarshal(data, &session); err != nil {
		return audioNotesSession{}, err
	}
	if session.ID == "" {
		session.ID = sessionID
	}
	if session.SessionDir == "" {
		session.SessionDir = filepath.Join(root, sessionID)
	}
	if session.RootDir == "" {
		session.RootDir = root
	}
	if session.Tracks == nil {
		session.Tracks = []audioTrackState{}
	}
	return session, nil
}

func saveAudioSession(session audioNotesSession) error {
	if strings.TrimSpace(session.ID) == "" {
		return errors.New("session id is required")
	}
	if strings.TrimSpace(session.SessionDir) == "" {
		return errors.New("session directory is required")
	}
	if strings.TrimSpace(session.RootDir) == "" {
		session.RootDir = filepath.Dir(session.SessionDir)
	}
	session.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := os.MkdirAll(session.SessionDir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(session.SessionDir, "session.json"), payload, 0o644)
}

func activeAudioSessionPath(root string) string {
	return filepath.Join(root, "active.json")
}

func loadActiveAudioSession(root string) (audioActiveSession, error) {
	path := activeAudioSessionPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return audioActiveSession{}, nil
		}
		return audioActiveSession{}, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return audioActiveSession{}, nil
	}
	var active audioActiveSession
	if err := json.Unmarshal(data, &active); err != nil {
		return audioActiveSession{}, err
	}
	return active, nil
}

func saveActiveAudioSession(root string, active audioActiveSession) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(activeAudioSessionPath(root), payload, 0o644)
}

func clearActiveAudioSession(root, sessionID string) {
	active, err := loadActiveAudioSession(root)
	if err != nil {
		return
	}
	if strings.TrimSpace(active.SessionID) == "" {
		return
	}
	if sessionID != "" && active.SessionID != sessionID {
		return
	}
	_ = os.Remove(activeAudioSessionPath(root))
}

func resolveAudioSources(micEnabled, speakerEnabled bool, micOverride, speakerOverride string) (string, string, error) {
	info, err := runAudioCommand("pactl", "info")
	if err != nil {
		return "", "", fmt.Errorf("failed to query pactl info: %w", err)
	}
	defaultMic := parsePactlInfoValue(info, "Default Source")
	defaultSink := parsePactlInfoValue(info, "Default Sink")

	mic := strings.TrimSpace(micOverride)
	if mic == "" {
		mic = defaultMic
	}
	speaker := strings.TrimSpace(speakerOverride)
	if speaker == "" && defaultSink != "" {
		speaker = defaultSink + ".monitor"
	}

	if micEnabled && mic == "" {
		return "", "", errors.New("unable to resolve microphone source; set --mic-source explicitly")
	}
	if speakerEnabled && speaker == "" {
		return "", "", errors.New("unable to resolve speaker monitor source; set --speaker-source explicitly")
	}
	return mic, speaker, nil
}

func parsePactlInfoValue(info, key string) string {
	needle := strings.ToLower(strings.TrimSpace(key)) + ":"
	for _, line := range strings.Split(info, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, needle) {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func startAudioCaptureProcess(source, outPath string) (int, error) {
	if strings.TrimSpace(source) == "" {
		return 0, errors.New("audio source is required")
	}
	if strings.TrimSpace(outPath) == "" {
		return 0, errors.New("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "pulse",
		"-i", source,
		"-ac", strconv.Itoa(audioCaptureChannels),
		"-ar", strconv.Itoa(audioCaptureSampleRate),
		"-acodec", "pcm_s16le",
		outPath,
	}
	cmd := tracedExecCommand("ffmpeg", args...)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

func stopAudioProcess(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if !isProcessAlive(pid) {
		return nil
	}

	_ = proc.Signal(syscall.SIGINT)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 12; i++ {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	_ = proc.Signal(syscall.SIGKILL)
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func runAudioCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := tracedExecCommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s command timed out", name)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(text)
	}
	return text, nil
}

func detectTranscriber(requested, modelValue, language string) (audioTranscriberSpec, bool, string) {
	choice := strings.ToLower(strings.TrimSpace(requested))
	if choice == "" {
		choice = "auto"
	}

	if choice == "none" {
		return audioTranscriberSpec{Name: "none"}, true, "transcription disabled by configuration"
	}

	if choice == "auto" || choice == "whisper" {
		if _, err := exec.LookPath("whisper"); err == nil {
			model := strings.TrimSpace(modelValue)
			if model == "" {
				if envModel := strings.TrimSpace(os.Getenv("WHISPER_MODEL")); envModel != "" {
					model = envModel
				} else {
					model = "base"
				}
			}
			return audioTranscriberSpec{Name: "whisper", Model: model}, true, "python whisper cli"
		}
		if choice == "whisper" {
			return audioTranscriberSpec{}, false, "whisper command not found"
		}
	}

	if choice == "auto" || choice == "whisper-cli" {
		if _, err := exec.LookPath("whisper-cli"); err == nil {
			model := strings.TrimSpace(modelValue)
			if model == "" {
				model = strings.TrimSpace(os.Getenv("WHISPER_CPP_MODEL"))
			}
			if model == "" {
				model = discoverWhisperCPPModelPath()
			}
			if model == "" {
				if choice == "whisper-cli" {
					return audioTranscriberSpec{}, false, "whisper-cli found but no model path provided (use --model or WHISPER_CPP_MODEL)"
				}
			} else {
				return audioTranscriberSpec{Name: "whisper-cli", Model: model}, true, "whisper.cpp cli"
			}
		}
		if choice == "whisper-cli" {
			return audioTranscriberSpec{}, false, "whisper-cli command not found"
		}
	}

	if choice == "auto" {
		if strings.TrimSpace(language) == "" {
			language = "en"
		}
		return audioTranscriberSpec{}, false, "no supported transcriber found (install `whisper` or `whisper-cli`)"
	}
	return audioTranscriberSpec{}, false, "unsupported transcriber choice"
}

func discoverWhisperCPPModelPath() string {
	candidates := []string{
		"models/ggml-base.en.bin",
		"models/ggml-base.bin",
		"/usr/share/whisper/models/ggml-base.en.bin",
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

func transcribeAudioTrack(spec audioTranscriberSpec, trackName, audioPath, sessionDir, language string) (string, error) {
	base := filepath.Join(sessionDir, trackName)
	switch spec.Name {
	case "whisper":
		args := []string{audioPath, "--model", spec.Model, "--output_format", "srt", "--output_dir", sessionDir}
		if strings.TrimSpace(language) != "" {
			args = append(args, "--language", strings.TrimSpace(language))
		}
		if _, err := runLongAudioCommand("whisper", args...); err != nil {
			return "", err
		}
		output := filepath.Join(sessionDir, strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))+".srt")
		if _, err := os.Stat(output); err != nil {
			return "", fmt.Errorf("whisper output missing: %s", output)
		}
		return output, nil
	case "whisper-cli":
		args := []string{"-m", spec.Model, "-f", audioPath, "-osrt", "-of", base}
		if strings.TrimSpace(language) != "" {
			args = append(args, "-l", strings.TrimSpace(language))
		}
		if _, err := runLongAudioCommand("whisper-cli", args...); err != nil {
			return "", err
		}
		output := base + ".srt"
		if _, err := os.Stat(output); err != nil {
			return "", fmt.Errorf("whisper-cli output missing: %s", output)
		}
		return output, nil
	default:
		return "", errors.New("unsupported transcriber backend")
	}
}

func runLongAudioCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	cmd := tracedExecCommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(truncateScreenText(text, 500))
	}
	return text, nil
}

func parseTranscriptSegments(source, srtPath string) ([]transcriptSegment, error) {
	lines, err := media.ParseSubtitleLines(srtPath)
	if err != nil {
		return nil, err
	}
	segments := make([]transcriptSegment, 0, len(lines))
	for _, line := range lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		segments = append(segments, transcriptSegment{
			Source: source,
			Start:  strings.TrimSpace(line.Start),
			End:    strings.TrimSpace(line.End),
			Text:   text,
		})
	}
	return segments, nil
}

func sortTranscriptSegments(segments []transcriptSegment) {
	sort.SliceStable(segments, func(i, j int) bool {
		left, leftOK := media.TimestampToMillis(segments[i].Start)
		right, rightOK := media.TimestampToMillis(segments[j].Start)
		if leftOK && rightOK {
			if left == right {
				return segments[i].Source < segments[j].Source
			}
			return left < right
		}
		if leftOK {
			return true
		}
		if rightOK {
			return false
		}
		if segments[i].Source == segments[j].Source {
			return segments[i].Text < segments[j].Text
		}
		return segments[i].Source < segments[j].Source
	})
}

func writeTranscriptSegments(path string, segments []transcriptSegment) error {
	payload, err := json.MarshalIndent(segments, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

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
