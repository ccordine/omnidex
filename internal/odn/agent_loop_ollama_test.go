package odn

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOllamaTerminalCapabilities(t *testing.T) {
	client := testOllamaClient(t)
	workspace := t.TempDir()

	cases := []struct {
		name        string
		objective   string
		wantCommand string
		assert      func(t *testing.T, result AgentCommandLoopResult)
	}{
		{
			name:        "pwd",
			objective:   "Run this exact command: pwd. Then finish from the observed stdout.",
			wantCommand: "pwd",
			assert: func(t *testing.T, result AgentCommandLoopResult) {
				if !transcriptStdoutContains(result.Transcript, workspace) {
					t.Fatalf("pwd stdout did not contain workspace %q\ntranscript: %#v", workspace, result.Transcript)
				}
			},
		},
		{
			name:        "date",
			objective:   "Run this exact command: date. Then finish from the observed stdout.",
			wantCommand: "date",
			assert: func(t *testing.T, result AgentCommandLoopResult) {
				if !transcriptStdoutHasAny(result.Transcript, []string{"2026", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}) {
					t.Fatalf("date stdout did not look like date output\ntranscript: %#v", result.Transcript)
				}
			},
		},
		{
			name:        "curl",
			objective:   "Run this exact command: curl --max-time 20 -s https://example.com | grep -oP '(?<=<title>).*?(?=</title>)'. Then finish from the observed stdout.",
			wantCommand: "curl",
			assert: func(t *testing.T, result AgentCommandLoopResult) {
				if !transcriptStdoutContains(result.Transcript, "Example Domain") {
					t.Fatalf("curl stdout did not contain Example Domain\ntranscript: %#v", result.Transcript)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runLogger, err := NewRunLogger(t.TempDir(), "ollama-capability-test")
			if err != nil {
				t.Fatal(err)
			}
			defer runLogger.Close()

			session := &Session{
				WorkspacePath: workspace,
				WorkspaceHash: "ollama-capability-test",
				Permission:    PermissionFull,
			}
			nextID := 0
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			result, err := ExecuteAgentCommandLoopWithConfig(
				ctx,
				session,
				tc.objective,
				PermissionFull,
				strings.NewReader(""),
				&bytes.Buffer{},
				client,
				func() string {
					nextID++
					return fmt.Sprintf("evt_%03d", nextID)
				},
				runLogger,
				AgentCommandLoopConfig{
					MaxSteps:            4,
					MaxCommandsPerStep:  2,
					MaxObservationChars: 1800,
					PlannerTimeout:      90 * time.Second,
					CommandTimeout:      30 * time.Second,
				},
			)
			if err != nil {
				if isOllamaRunnerStoppedError(err) {
					t.Skipf("Ollama runner stopped during terminal capability test: %v", err)
				}
				t.Fatal(err)
			}
			if result.ExecutedCount == 0 {
				if isOllamaRunnerStoppedSummary(result.Summary, result.Events) {
					t.Skipf("Ollama runner stopped during terminal capability test: %s", result.Summary)
				}
				t.Fatalf("executed no commands; summary=%q transcript=%#v events=%#v", result.Summary, result.Transcript, result.Events)
			}
			if !transcriptCommandContains(result.Transcript, tc.wantCommand) {
				t.Fatalf("did not execute command containing %q\ntranscript: %#v", tc.wantCommand, result.Transcript)
			}
			tc.assert(t, result)
		})
	}
}

func testOllamaClient(t *testing.T) *OllamaClient {
	t.Helper()

	endpoint := strings.TrimSpace(os.Getenv("ODN_OLLAMA_ENDPOINT"))
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}
	model := strings.TrimSpace(os.Getenv("ODN_OLLAMA_MODEL"))
	if model == "" {
		model = defaultOllamaModel
	}

	client := NewOllamaClient(endpoint, model)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err := client.ChatRaw(ctx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: "Output exactly: OK"},
			{Role: "user", Content: "health check"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 8,
		},
	})
	if err != nil {
		t.Skipf("Ollama model %q unavailable at %s: %v", model, endpoint, err)
	}
	t.Logf("using live Ollama model %q at %s", model, endpoint)
	return client
}

func transcriptCommandContains(transcript []CommandObservation, needle string) bool {
	for _, obs := range transcript {
		if strings.Contains(obs.Command, needle) {
			return true
		}
	}
	return false
}

func transcriptStdoutContains(transcript []CommandObservation, needle string) bool {
	for _, obs := range transcript {
		if strings.Contains(obs.Stdout, needle) {
			return true
		}
	}
	return false
}

func transcriptStdoutHasAny(transcript []CommandObservation, needles []string) bool {
	for _, needle := range needles {
		if transcriptStdoutContains(transcript, needle) {
			return true
		}
	}
	return false
}
