package omni

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

type RunLogger struct {
	mu       sync.Mutex
	runID    string
	rootDir  string
	filePath string
	file     *os.File
	sequence int
}

type LogRecord struct {
	RunID     string                 `json:"run_id"`
	Sequence  int                    `json:"sequence"`
	Timestamp string                 `json:"timestamp"`
	Component string                 `json:"component"`
	EventType string                 `json:"event_type"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func NewRunLogger(rootDir, workspaceHash string) (*RunLogger, error) {
	if strings.TrimSpace(workspaceHash) == "" {
		return nil, errors.New("workspace hash is required for run logger")
	}

	base := strings.TrimSpace(rootDir)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			base = ".omni/runs"
		} else {
			base = filepath.Join(home, ".omni", "runs")
		}
	}

	workspaceDir := filepath.Join(base, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create run log directory: %w", err)
	}

	runID := newRunID()
	filePath := filepath.Join(workspaceDir, runID+".jsonl")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create run log file: %w", err)
	}

	logger := &RunLogger{
		runID:    runID,
		rootDir:  base,
		filePath: filePath,
		file:     f,
	}
	if err := logger.Log("runtime", "run_started", map[string]interface{}{
		"workspace_hash": workspaceHash,
		"log_path":       filePath,
	}); err != nil {
		_ = f.Close()
		return nil, err
	}

	return logger, nil
}

func (l *RunLogger) RunID() string {
	if l == nil {
		return ""
	}
	return l.runID
}

func (l *RunLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.filePath
}

func (l *RunLogger) Log(component, eventType string, fields map[string]interface{}) error {
	if l == nil {
		return nil
	}
	if strings.TrimSpace(component) == "" {
		return errors.New("log component is required")
	}
	if strings.TrimSpace(eventType) == "" {
		return errors.New("log event type is required")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return errors.New("run logger is closed")
	}

	l.sequence++
	record := LogRecord{
		RunID:     l.runID,
		Sequence:  l.sequence,
		Timestamp: nowUTC(),
		Component: component,
		EventType: eventType,
		Fields:    fields,
	}

	blob, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode log record: %w", err)
	}

	if _, err := l.file.Write(append(blob, '\n')); err != nil {
		return fmt.Errorf("append run log record: %w", err)
	}
	return nil
}

func (l *RunLogger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	l.sequence++
	finalRecord := LogRecord{
		RunID:     l.runID,
		Sequence:  l.sequence,
		Timestamp: nowUTC(),
		Component: "runtime",
		EventType: "run_completed",
		Fields: map[string]interface{}{
			"record_count": l.sequence,
		},
	}
	if blob, err := json.Marshal(finalRecord); err == nil {
		_, _ = l.file.Write(append(blob, '\n'))
	}

	err := l.file.Close()
	l.file = nil
	if err != nil {
		return fmt.Errorf("close run log: %w", err)
	}
	return nil
}

func newRunID() string {
	return "run_" + time.Now().UTC().Format("20060102_150405") + "_" + shortNanoSuffix()
}

func shortNanoSuffix() string {
	v := time.Now().UTC().UnixNano()
	if v < 0 {
		v = -v
	}
	return fmt.Sprintf("%06d", v%1000000)
}
