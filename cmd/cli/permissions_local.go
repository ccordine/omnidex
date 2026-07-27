package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const permissionStoreEnv = "OMNI_PERMISSIONS_PATH"

const (
	permissionKeyMediaControl    = "local.media.control"
	permissionKeyMediaRead       = "local.media.read"
	permissionKeyBrowserInspect  = "local.browser.inspect"
	permissionKeyBrowserConsole  = "local.browser.console"
	permissionKeyScreenCapture   = "local.screen.capture"
	permissionKeyScreenOCR       = "local.screen.ocr"
	permissionKeyScreenVision    = "local.screen.vision"
	permissionKeyShellExec       = "local.shell.exec"
	permissionKeyShellSudo       = "local.shell.sudo"
	permissionKeyAudioMic        = "local.audio.mic"
	permissionKeyAudioSpeaker    = "local.audio.speaker"
	permissionKeyAudioTranscribe = "local.audio.transcribe"
)

type permissionDecision struct {
	Allowed   bool   `json:"allowed"`
	UpdatedAt string `json:"updated_at"`
	Reason    string `json:"reason,omitempty"`
}

type permissionRegistry struct {
	Version     int                           `json:"version"`
	UpdatedAt   string                        `json:"updated_at"`
	Permissions map[string]permissionDecision `json:"permissions"`
}

type permissionManager struct {
	mu     sync.Mutex
	path   string
	loaded bool
	state  permissionRegistry
}

var (
	permissionManagerOnce sync.Once
	permissionManagerInst *permissionManager
)

type permissionPromptFunc func(key, reason, storePath, description string) (bool, error)

var (
	permissionPromptOverrideMu sync.RWMutex
	permissionPromptOverride   permissionPromptFunc
)

func installPermissionPromptFunc(fn permissionPromptFunc) func() {
	permissionPromptOverrideMu.Lock()
	previous := permissionPromptOverride
	permissionPromptOverride = fn
	permissionPromptOverrideMu.Unlock()
	return func() {
		permissionPromptOverrideMu.Lock()
		permissionPromptOverride = previous
		permissionPromptOverrideMu.Unlock()
	}
}

func currentPermissionPromptFunc() permissionPromptFunc {
	permissionPromptOverrideMu.RLock()
	defer permissionPromptOverrideMu.RUnlock()
	return permissionPromptOverride
}

var knownPermissionDescriptions = map[string]string{
	permissionKeyMediaControl:    "Control local media playback and open files in VLC/player.",
	permissionKeyMediaRead:       "Read local media player metadata (title, path, playback timestamp).",
	permissionKeyBrowserInspect:  "Inspect local browser processes and tab metadata (when available).",
	permissionKeyBrowserConsole:  "Read JavaScript console events from browser DevTools endpoints.",
	permissionKeyScreenCapture:   "Capture a screenshot of the active display.",
	permissionKeyScreenOCR:       "Run OCR on captured screenshots to extract visible text.",
	permissionKeyScreenVision:    "Send captured screenshots to local Ollama vision model for interpretation.",
	permissionKeyShellExec:       "Execute local shell commands and file operations from chat automation.",
	permissionKeyShellSudo:       "Allow elevated local shell commands via sudo after interactive authentication.",
	permissionKeyAudioMic:        "Capture microphone audio for long-running notes.",
	permissionKeyAudioSpeaker:    "Capture speaker/monitor audio for long-running notes.",
	permissionKeyAudioTranscribe: "Transcribe captured audio into text notes and quotes.",
}

func getPermissionManager() *permissionManager {
	permissionManagerOnce.Do(func() {
		permissionManagerInst = &permissionManager{
			path: strings.TrimSpace(defaultPermissionStorePath()),
			state: permissionRegistry{
				Version:     1,
				Permissions: map[string]permissionDecision{},
			},
		}
	})
	return permissionManagerInst
}

func defaultPermissionStorePath() string {
	if raw := strings.TrimSpace(os.Getenv(permissionStoreEnv)); raw != "" {
		return raw
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "omni", "permissions.json")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".config", "omni", "permissions.json")
	}
	return filepath.Join(".omni", "permissions.json")
}

func fallbackPermissionStorePath() string {
	return filepath.Join(".omni", "permissions.json")
}

func ensureLocalPermission(key, reason string) error {
	allowed, err := getPermissionManager().Require(key, reason)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("permission denied for %s (manage with `omni permissions grant %s`)", key, key)
	}
	return nil
}

func (pm *permissionManager) Require(key, reason string) (bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, errors.New("permission key is required")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := pm.loadLocked(); err != nil {
		return false, err
	}
	if decision, ok := pm.state.Permissions[key]; ok {
		return decision.Allowed, nil
	}

	allowed, err := promptPermissionDecision(key, reason, pm.path)
	if err != nil {
		return false, err
	}

	pm.state.Permissions[key] = permissionDecision{
		Allowed:   allowed,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Reason:    strings.TrimSpace(reason),
	}
	pm.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := pm.saveLocked(); err != nil {
		return false, err
	}
	return allowed, nil
}

func (pm *permissionManager) List() (string, map[string]permissionDecision, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := pm.loadLocked(); err != nil {
		return pm.path, nil, err
	}
	out := make(map[string]permissionDecision, len(pm.state.Permissions))
	for key, value := range pm.state.Permissions {
		out[key] = value
	}
	return pm.path, out, nil
}

func (pm *permissionManager) Set(key string, allowed bool, reason string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("permission key is required")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := pm.loadLocked(); err != nil {
		return err
	}
	pm.state.Permissions[key] = permissionDecision{
		Allowed:   allowed,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Reason:    strings.TrimSpace(reason),
	}
	pm.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return pm.saveLocked()
}

func (pm *permissionManager) Unset(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("permission key is required")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := pm.loadLocked(); err != nil {
		return err
	}
	delete(pm.state.Permissions, key)
	pm.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return pm.saveLocked()
}

func (pm *permissionManager) Reset() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := pm.loadLocked(); err != nil {
		return err
	}
	pm.state.Permissions = map[string]permissionDecision{}
	pm.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return pm.saveLocked()
}

func (pm *permissionManager) loadLocked() error {
	if pm.loaded {
		if pm.state.Permissions == nil {
			pm.state.Permissions = map[string]permissionDecision{}
		}
		if pm.state.Version == 0 {
			pm.state.Version = 1
		}
		return nil
	}

	pm.loaded = true
	pm.state = permissionRegistry{
		Version:     1,
		Permissions: map[string]permissionDecision{},
	}

	candidates := []string{pm.path}
	if fallback := strings.TrimSpace(fallbackPermissionStorePath()); fallback != "" && fallback != pm.path {
		candidates = append(candidates, fallback)
	}

	var lastErr error
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			lastErr = err
			continue
		}
		if strings.TrimSpace(string(data)) == "" {
			pm.path = candidate
			return nil
		}

		if err := json.Unmarshal(data, &pm.state); err != nil {
			lastErr = err
			continue
		}
		pm.path = candidate
		if pm.state.Permissions == nil {
			pm.state.Permissions = map[string]permissionDecision{}
		}
		if pm.state.Version == 0 {
			pm.state.Version = 1
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

func (pm *permissionManager) saveLocked() error {
	candidates := []string{pm.path}
	if fallback := strings.TrimSpace(fallbackPermissionStorePath()); fallback != "" && fallback != pm.path {
		candidates = append(candidates, fallback)
	}

	var lastErr error
	for _, candidate := range candidates {
		if err := writePermissionRegistry(candidate, pm.state); err != nil {
			lastErr = err
			continue
		}
		pm.path = candidate
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("unable to write permission registry")
}

func writePermissionRegistry(path string, state permissionRegistry) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}
