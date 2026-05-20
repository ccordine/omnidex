package omni

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CommandCacheEntry struct {
	Version   string `json:"version"`
	Key       string `json:"key"`
	Workspace string `json:"workspace"`
	Command   string `json:"command"`
	InputHash string `json:"input_hash"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	CreatedAt string `json:"created_at"`
}

func CommandCacheKey(index WorkspaceIndex, command string) string {
	inputHash := CommandCacheInputHash(index)
	sum := sha256.Sum256([]byte(index.Workspace + "\x00" + strings.TrimSpace(command) + "\x00" + inputHash))
	return hex.EncodeToString(sum[:])
}

func CommandCacheInputHash(index WorkspaceIndex) string {
	parts := make([]string, 0, len(index.Files))
	for _, file := range index.Files {
		parts = append(parts, file.Path+"="+file.SHA256)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func SaveCommandCacheEntry(root string, entry CommandCacheEntry) error {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(entry.Workspace, ".omni", "command-cache")
	}
	if strings.TrimSpace(entry.Key) == "" {
		return fmt.Errorf("command cache key is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create command cache root: %w", err)
	}
	entry.Version = "1.0"
	if entry.CreatedAt == "" {
		entry.CreatedAt = nowUTC()
	}
	blob, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode command cache entry: %w", err)
	}
	return os.WriteFile(filepath.Join(root, entry.Key+".json"), append(blob, '\n'), 0o644)
}

func LoadCommandCacheEntry(root, key string) (CommandCacheEntry, bool, error) {
	if strings.TrimSpace(root) == "" {
		return CommandCacheEntry{}, false, fmt.Errorf("command cache root is required")
	}
	blob, err := os.ReadFile(filepath.Join(root, strings.TrimSpace(key)+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return CommandCacheEntry{}, false, nil
		}
		return CommandCacheEntry{}, false, err
	}
	var entry CommandCacheEntry
	if err := json.Unmarshal(blob, &entry); err != nil {
		return CommandCacheEntry{}, false, fmt.Errorf("decode command cache entry: %w", err)
	}
	return entry, true, nil
}
