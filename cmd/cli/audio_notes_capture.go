package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

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
