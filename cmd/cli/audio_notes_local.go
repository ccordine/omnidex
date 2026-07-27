package main

import (
	"flag"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
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
