package omni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EvidenceLedger struct {
	Version     string               `json:"version"`
	Workspace   string               `json:"workspace"`
	WorkspaceID string               `json:"workspace_id"`
	GeneratedAt string               `json:"generated_at"`
	Turns       []EvidenceLedgerTurn `json:"turns"`
	Summary     EvidenceSummary      `json:"summary"`
}

type EvidenceLedgerTurn struct {
	ID               string                   `json:"id"`
	UserInput        string                   `json:"user_input"`
	Response         string                   `json:"response"`
	CreatedAt        string                   `json:"created_at"`
	Objectives       []string                 `json:"objectives,omitempty"`
	Pending          []string                 `json:"pending,omitempty"`
	Commands         []EvidenceLedgerCommand  `json:"commands,omitempty"`
	RejectedCommands []EvidenceLedgerRejected `json:"rejected_commands,omitempty"`
	Events           []Event                  `json:"events"`
}

type EvidenceLedgerCommand struct {
	Step     string `json:"step,omitempty"`
	Command  string `json:"command"`
	ExitCode string `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type EvidenceLedgerRejected struct {
	Step    string `json:"step,omitempty"`
	Command string `json:"command,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type EvidenceSummary struct {
	TurnCount            int `json:"turn_count"`
	CommandCount         int `json:"command_count"`
	RejectedCommandCount int `json:"rejected_command_count"`
	FailedTurnCount      int `json:"failed_turn_count"`
	ModelCallCount       int `json:"model_call_count"`
	ModelFailureCount    int `json:"model_failure_count"`
	DoneRejectionCount   int `json:"done_rejection_count"`
	LoopExhaustionCount  int `json:"loop_exhaustion_count"`
}

func BuildEvidenceLedger(session *Session) EvidenceLedger {
	if session == nil {
		return EvidenceLedger{Version: "1.0", GeneratedAt: nowUTC()}
	}
	ledger := EvidenceLedger{
		Version:     "1.0",
		Workspace:   session.WorkspacePath,
		WorkspaceID: session.WorkspaceHash,
		GeneratedAt: nowUTC(),
		Turns:       make([]EvidenceLedgerTurn, 0, len(session.Turns)),
	}
	for _, turn := range session.Turns {
		item := EvidenceLedgerTurn{
			ID:        turn.ID,
			UserInput: turn.UserInput,
			Response:  turn.Response,
			CreatedAt: turn.CreatedAt,
			Events:    turn.Events,
		}
		for _, event := range turn.Events {
			switch event.Type {
			case "prompt_interpreter_completed", "structured_llm_request_started", "structured_llm_payload_received", "structured_done_rejected", "completion_check_completed":
				if pending := strings.TrimSpace(event.Details["pending_objectives"]); pending != "" {
					item.Pending = splitLedgerCSV(pending)
				}
			case "structured_command_finished", "structured_command_completed":
				cmd := strings.TrimSpace(event.Details["command"])
				if cmd != "" {
					item.Commands = append(item.Commands, EvidenceLedgerCommand{
						Step:     event.Details["step"],
						Command:  cmd,
						ExitCode: event.Details["exit_code"],
						Stdout:   event.Details["stdout"],
						Stderr:   event.Details["stderr"],
					})
				}
			case "structured_command_rejected":
				item.RejectedCommands = append(item.RejectedCommands, EvidenceLedgerRejected{
					Step:    event.Details["step"],
					Command: event.Details["command"],
					Reason:  event.Details["reason"],
				})
			case "structured_loop_exhausted", "structured_command_failed", "structured_planner_failed_after_progress":
				ledger.Summary.FailedTurnCount++
			}
			updateEvidenceSummaryCounts(event.Type, &ledger.Summary)
		}
		ledger.Summary.CommandCount += len(item.Commands)
		ledger.Summary.RejectedCommandCount += len(item.RejectedCommands)
		ledger.Turns = append(ledger.Turns, item)
	}
	ledger.Summary.TurnCount = len(ledger.Turns)
	return ledger
}

func updateEvidenceSummaryCounts(eventType string, summary *EvidenceSummary) {
	if summary == nil {
		return
	}
	switch eventType {
	case "structured_llm_request_started", "prompt_interpreter_completed", "minimal_context_updated", "completion_check_completed":
		summary.ModelCallCount++
	case "structured_llm_request_failed", "prompt_interpreter_failed", "minimal_context_failed", "completion_check_failed":
		summary.ModelFailureCount++
	case "structured_done_rejected":
		summary.DoneRejectionCount++
	case "structured_loop_exhausted":
		summary.LoopExhaustionCount++
	}
}

func ExportEvidenceLedger(session *Session, outputPath string) error {
	ledger := BuildEvidenceLedger(session)
	blob, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence ledger: %w", err)
	}
	target := strings.TrimSpace(outputPath)
	if target == "" || target == "-" {
		_, err = os.Stdout.Write(append(blob, '\n'))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create ledger output directory: %w", err)
	}
	if err := os.WriteFile(target, append(blob, '\n'), 0o644); err != nil {
		return fmt.Errorf("write evidence ledger: %w", err)
	}
	return nil
}

func splitLedgerCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
