package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BenchmarkManifest struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	Workspace       string   `json:"workspace"`
	Prompt          string   `json:"prompt"`
	Recipe          string   `json:"recipe,omitempty"`
	SuccessCriteria []string `json:"success_criteria"`
}

type BenchmarkReport struct {
	Workspace        string   `json:"workspace"`
	WorkspaceID      string   `json:"workspace_id"`
	GeneratedAt      string   `json:"generated_at"`
	Turns            int      `json:"turns"`
	ModelCalls       int      `json:"model_calls"`
	ModelFailures    int      `json:"model_failures"`
	Commands         int      `json:"commands"`
	CommandFailures  int      `json:"command_failures"`
	RejectedCommands int      `json:"rejected_commands"`
	DoneRejections   int      `json:"done_rejections"`
	LoopExhaustions  int      `json:"loop_exhaustions"`
	Duration         string   `json:"duration,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

type BenchmarkRunResult struct {
	ID              string          `json:"id"`
	Description     string          `json:"description"`
	Workspace       string          `json:"workspace"`
	StartedAt       string          `json:"started_at"`
	FinishedAt      string          `json:"finished_at"`
	Duration        string          `json:"duration"`
	Success         bool            `json:"success"`
	Error           string          `json:"error,omitempty"`
	SuccessCriteria []string        `json:"success_criteria"`
	Report          BenchmarkReport `json:"report"`
}

type BenchmarkRunOptions struct {
	Root        string
	Workspace   string
	SessionRoot string
	DryRun      bool
}

func BenchmarkReportFromSession(session *Session) BenchmarkReport {
	trace := BuildRunTrace(session)
	report := BenchmarkReport{
		Workspace:        trace.Workspace,
		WorkspaceID:      trace.WorkspaceID,
		GeneratedAt:      trace.GeneratedAt,
		Turns:            trace.TurnCount,
		ModelCalls:       trace.ModelCalls,
		ModelFailures:    trace.ModelFailures,
		Commands:         trace.Commands,
		CommandFailures:  trace.CommandFailures,
		RejectedCommands: trace.RejectedCommands,
		DoneRejections:   trace.DoneRejections,
		LoopExhaustions:  trace.LoopExhaustions,
		Duration:         trace.EstimatedDuration,
	}
	if report.LoopExhaustions > 0 {
		report.Warnings = append(report.Warnings, "structured loop exhausted")
	}
	if report.RejectedCommands > report.Commands && report.RejectedCommands > 0 {
		report.Warnings = append(report.Warnings, "more rejected commands than executed commands")
	}
	if report.ModelFailures > 0 {
		report.Warnings = append(report.Warnings, "model request failures observed")
	}
	return report
}

func formatBenchmarkReportText(report BenchmarkReport) string {
	lines := []string{
		fmt.Sprintf("workspace=%s", report.Workspace),
		fmt.Sprintf("turns=%d duration=%s", report.Turns, emptyAs(report.Duration, "unknown")),
		fmt.Sprintf("model_calls=%d model_failures=%d", report.ModelCalls, report.ModelFailures),
		fmt.Sprintf("commands=%d command_failures=%d rejected_commands=%d done_rejections=%d loop_exhaustions=%d", report.Commands, report.CommandFailures, report.RejectedCommands, report.DoneRejections, report.LoopExhaustions),
	}
	if len(report.Warnings) > 0 {
		lines = append(lines, "warnings="+strings.Join(report.Warnings, "; "))
	}
	return strings.Join(lines, "\n")
}

func RunBenchmarkManifest(ctx context.Context, manifest BenchmarkManifest, client CommandDecisionClient, stdout, stderr io.Writer, opts BenchmarkRunOptions) (BenchmarkRunResult, error) {
	start := time.Now()
	result := BenchmarkRunResult{
		ID:              manifest.ID,
		Description:     manifest.Description,
		StartedAt:       start.UTC().Format(time.RFC3339),
		SuccessCriteria: append([]string(nil), manifest.SuccessCriteria...),
	}
	workspace, err := prepareBenchmarkWorkspace(manifest, opts)
	if err != nil {
		result.Error = err.Error()
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result, err
	}
	result.Workspace = workspace
	if opts.DryRun {
		result.Success = true
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result, nil
	}
	store := NewSessionStore(opts.SessionRoot)
	session, _, err := store.LoadOrCreate(workspace)
	if err != nil {
		result.Error = err.Error()
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result, err
	}
	session.Permission = PermissionFull
	session.ActiveDirectoryPath = workspace
	if err := store.Save(session); err != nil {
		result.Error = err.Error()
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		return result, err
	}
	events := []Event{}
	commandResult, runErr := runStructuredCommandDecisionWithConfig(
		ctx,
		manifest.Prompt,
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, Event{
				Type:      evt.Type,
				Summary:   evt.Summary,
				Details:   evt.Details,
				CreatedAt: nowUTC(),
			})
		},
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	turn := Turn{
		ID:                   "bench_000001",
		UserInput:            manifest.Prompt,
		IntentClassification: IntentExecution,
		Confidence:           1,
		Response:             commandResult.Answer,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	if runErr != nil {
		result.Error = runErr.Error()
		turn.Response = runErr.Error()
		turn.Events = append(turn.Events, Event{Type: "structured_command_failed", Summary: "Benchmark structured command failed", Details: map[string]string{"error": runErr.Error()}, CreatedAt: nowUTC()})
	} else {
		turn.Events = append(turn.Events, Event{Type: "structured_command_completed", Summary: "Benchmark structured command completed", Details: map[string]string{
			"command":   commandResult.Command,
			"exit_code": fmt.Sprintf("%d", commandResult.ExitCode),
			"stdout":    commandObservationStdout(commandResult),
			"stderr":    commandObservationStderr(commandResult),
		}, CreatedAt: nowUTC()})
	}
	session.Turns = append(session.Turns, turn)
	if err := store.Save(session); err != nil && runErr == nil {
		result.Error = err.Error()
		runErr = err
	}
	result.Report = BenchmarkReportFromSession(session)
	result.Success = runErr == nil && benchmarkSuccessCriteriaObserved(manifest, workspace, commandResult)
	if !result.Success && result.Error == "" {
		result.Error = "success criteria were not fully observed"
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	result.Duration = time.Since(start).Round(time.Millisecond).String()
	return result, runErr
}

func prepareBenchmarkWorkspace(manifest BenchmarkManifest, opts BenchmarkRunOptions) (string, error) {
	if strings.TrimSpace(opts.Workspace) != "" {
		if err := os.MkdirAll(opts.Workspace, 0o755); err != nil {
			return "", fmt.Errorf("create benchmark workspace: %w", err)
		}
		return filepath.Abs(opts.Workspace)
	}
	mode := strings.TrimSpace(manifest.Workspace)
	if mode == "" || mode == "tmp" {
		root := strings.TrimSpace(opts.Root)
		if root == "" {
			root = filepath.Join(os.TempDir(), "omni-bench")
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", fmt.Errorf("create benchmark root: %w", err)
		}
		return os.MkdirTemp(root, sanitizeBenchmarkID(manifest.ID)+"-")
	}
	if filepath.IsAbs(mode) || strings.HasPrefix(mode, ".") {
		if err := os.MkdirAll(mode, 0o755); err != nil {
			return "", fmt.Errorf("create benchmark workspace: %w", err)
		}
		return filepath.Abs(mode)
	}
	return "", fmt.Errorf("unsupported benchmark workspace mode %q", manifest.Workspace)
}

func sanitizeBenchmarkID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "benchmark"
	}
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func commandObservationStdout(result CommandDecisionResult) string {
	if len(result.Observations) == 0 {
		return ""
	}
	return result.Observations[len(result.Observations)-1].Stdout
}

func commandObservationStderr(result CommandDecisionResult) string {
	if len(result.Observations) == 0 {
		return ""
	}
	return result.Observations[len(result.Observations)-1].Stderr
}

func benchmarkSuccessCriteriaObserved(manifest BenchmarkManifest, workspace string, result CommandDecisionResult) bool {
	if strings.TrimSpace(result.Command) == "" || result.ExitCode != 0 {
		return false
	}
	for _, criterion := range manifest.SuccessCriteria {
		if !benchmarkCriterionObserved(criterion, workspace, result) {
			return false
		}
	}
	return true
}

func benchmarkCriterionObserved(criterion, workspace string, result CommandDecisionResult) bool {
	clean := strings.ToLower(strings.TrimSpace(criterion))
	if clean == "" {
		return true
	}
	if strings.Contains(clean, "package.json exists") {
		return fileExists(filepath.Join(workspace, "package.json"))
	}
	if strings.Contains(clean, "webpack.config.js exists") {
		return fileExists(filepath.Join(workspace, "webpack.config.js"))
	}
	if strings.Contains(clean, "dist/bundle.js exists") {
		return fileExists(filepath.Join(workspace, "dist", "bundle.js"))
	}
	if strings.Contains(clean, "src files exist") {
		return dirHasFiles(filepath.Join(workspace, "src"))
	}
	if strings.Contains(clean, "final objective ledger has no pending objectives") {
		return pendingStructuredObjectiveIDs(result.ObjectiveLedger) == ""
	}
	if strings.Contains(clean, "package.json records ") {
		name := strings.TrimSpace(strings.TrimPrefix(clean, "package.json records "))
		blob, err := os.ReadFile(filepath.Join(workspace, "package.json"))
		return err == nil && strings.Contains(strings.ToLower(string(blob)), name)
	}
	return true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirHasFiles(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return true
		}
	}
	return false
}

func LoadBenchmarkManifests(root string) ([]BenchmarkManifest, error) {
	dir := strings.TrimSpace(root)
	if dir == "" {
		dir = "benchmarks"
	}
	manifests := []BenchmarkManifest{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "benchmark.json" {
			return nil
		}
		manifest, err := LoadBenchmarkManifest(path)
		if err != nil {
			return err
		}
		manifests = append(manifests, manifest)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load benchmark manifests: %w", err)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	return manifests, nil
}

func LoadBenchmarkManifest(path string) (BenchmarkManifest, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return BenchmarkManifest{}, fmt.Errorf("read benchmark %s: %w", path, err)
	}
	var manifest BenchmarkManifest
	if err := json.Unmarshal(blob, &manifest); err != nil {
		return BenchmarkManifest{}, fmt.Errorf("decode benchmark %s: %w", path, err)
	}
	if err := validateBenchmarkManifest(manifest); err != nil {
		return BenchmarkManifest{}, fmt.Errorf("invalid benchmark %s: %w", path, err)
	}
	return manifest, nil
}

func validateBenchmarkManifest(manifest BenchmarkManifest) error {
	if strings.TrimSpace(manifest.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(manifest.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if strings.TrimSpace(manifest.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if len(manifest.SuccessCriteria) == 0 {
		return fmt.Errorf("success_criteria is required")
	}
	return nil
}
