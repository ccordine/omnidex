package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/media"
)

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
