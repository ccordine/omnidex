package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultProjectProfileTimeout  = 45 * time.Second
	defaultProjectProfileMaxFiles = 120
)

type ProjectRunProfileConfig struct {
	Timeout  time.Duration
	MaxFiles int
}

type ProjectRunProfile struct {
	Summary       string   `json:"summary"`
	Languages     []string `json:"languages"`
	Frameworks    []string `json:"frameworks"`
	RunCommands   []string `json:"run_commands"`
	TestCommands  []string `json:"test_commands"`
	BuildCommands []string `json:"build_commands"`
	Evidence      []string `json:"evidence"`
}

type projectRunProfilePayload struct {
	Summary       *string  `json:"summary"`
	Languages     []string `json:"languages"`
	Frameworks    []string `json:"frameworks"`
	RunCommands   []string `json:"run_commands"`
	TestCommands  []string `json:"test_commands"`
	BuildCommands []string `json:"build_commands"`
	Evidence      []string `json:"evidence"`
}

func BuildProjectRunProfile(ctx context.Context, workspacePath string, client CommandDecisionClient, cfg ProjectRunProfileConfig) (ProjectRunProfile, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return ProjectRunProfile{}, fmt.Errorf("workspace path is required")
	}
	if client == nil {
		return ProjectRunProfile{}, fmt.Errorf("llm client is required")
	}
	cfg = normalizeProjectRunProfileConfig(cfg)
	snapshot, err := BuildProjectWorkspaceSnapshot(workspacePath, cfg.MaxFiles)
	if err != nil {
		return ProjectRunProfile{}, err
	}

	profileCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	resp, err := client.ChatRaw(profileCtx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: buildProjectRunProfilerPrompt()},
			{Role: "user", Content: snapshot},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"summary":        map[string]interface{}{"type": "string"},
				"languages":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"frameworks":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"run_commands":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"test_commands":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"build_commands": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"evidence":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"summary", "languages", "frameworks", "run_commands", "test_commands", "build_commands", "evidence"},
		},
		Options: map[string]interface{}{"temperature": 0},
	})
	if err != nil {
		return ProjectRunProfile{}, err
	}
	return parseProjectRunProfile(resp.Content)
}

func normalizeProjectRunProfileConfig(cfg ProjectRunProfileConfig) ProjectRunProfileConfig {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultProjectProfileTimeout
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = defaultProjectProfileMaxFiles
	}
	return cfg
}

func buildProjectRunProfilerPrompt() string {
	return strings.Join(withMinimalOutputContract(
		"Role: project run profiler.",
		"Infer how this project is built, tested, and run from workspace evidence only.",
		"Do not guess commands that are not supported by evidence.",
		"If evidence is insufficient, return empty command arrays and explain uncertainty in summary.",
		"Prefer exact commands a terminal agent can execute.",
		"Output JSON only.",
		"Schema: {\"summary\":\"...\",\"languages\":[\"...\"],\"frameworks\":[\"...\"],\"run_commands\":[\"...\"],\"test_commands\":[\"...\"],\"build_commands\":[\"...\"],\"evidence\":[\"...\"]}",
	), "\n")
}

func BuildProjectWorkspaceSnapshot(workspacePath string, maxFiles int) (string, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	if maxFiles <= 0 {
		maxFiles = defaultProjectProfileMaxFiles
	}
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", err
	}

	files := []string{}
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == abs {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() && shouldSkipSnapshotDir(name) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			files = append(files, rel+"/")
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	return "Workspace: " + abs + "\nFiles:\n" + strings.Join(files, "\n"), nil
}

func shouldSkipSnapshotDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "target", "build", "dist", ".next", ".cache":
		return true
	default:
		return false
	}
}

func parseProjectRunProfile(raw string) (ProjectRunProfile, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var payload projectRunProfilePayload
	if err := json.Unmarshal([]byte(clean), &payload); err != nil {
		return ProjectRunProfile{}, fmt.Errorf("parse project run profile JSON: %w", err)
	}
	if payload.Summary == nil {
		return ProjectRunProfile{}, fmt.Errorf("project run profile missing summary")
	}
	return ProjectRunProfile{
		Summary:       strings.TrimSpace(*payload.Summary),
		Languages:     cleanProfileList(payload.Languages),
		Frameworks:    cleanProfileList(payload.Frameworks),
		RunCommands:   cleanProfileList(payload.RunCommands),
		TestCommands:  cleanProfileList(payload.TestCommands),
		BuildCommands: cleanProfileList(payload.BuildCommands),
		Evidence:      cleanProfileList(payload.Evidence),
	}, nil
}

func cleanProfileList(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func formatProjectRunProfile(profile ProjectRunProfile) string {
	parts := []string{
		"Summary: " + profile.Summary,
		"Languages: " + strings.Join(profile.Languages, ", "),
		"Frameworks: " + strings.Join(profile.Frameworks, ", "),
		"Run commands: " + strings.Join(profile.RunCommands, " | "),
		"Test commands: " + strings.Join(profile.TestCommands, " | "),
		"Build commands: " + strings.Join(profile.BuildCommands, " | "),
		"Evidence: " + strings.Join(profile.Evidence, " | "),
	}
	return strings.Join(parts, "\n")
}
