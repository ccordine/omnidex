package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
