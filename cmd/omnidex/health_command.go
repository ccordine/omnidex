package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	localCoreHealthURL      = "http://127.0.0.1:8090/healthz"
	localCoreHealthMaxBytes = 256 * 1024
	localCoreHealthTimeout  = 5 * time.Second
)

type runningCoreHealth struct {
	Status       string                           `json:"status"`
	Time         time.Time                        `json:"time"`
	QueueEnabled bool                             `json:"queue_enabled"`
	ListenAddr   string                           `json:"listen_addr"`
	Release      json.RawMessage                  `json:"release"`
	Dependencies map[string]runningCoreDependency `json:"dependencies"`
}

type runningCoreDependency struct {
	Status     string `json:"status"`
	Configured bool   `json:"configured"`
	Required   bool   `json:"required"`
	Reachable  bool   `json:"reachable"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Target     string `json:"target,omitempty"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

type runningCoreHealthVerifier func(
	context.Context,
	*http.Client,
	string,
) error

func runCommand(args []string, stdin io.Reader, stdout io.Writer) error {
	return runCommandWithVerifier(args, stdin, stdout, verifyRunningCoreHealth)
}

func runCommandWithVerifier(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	verify runningCoreHealthVerifier,
) error {
	if stdout == nil {
		return fmt.Errorf("command output is unavailable")
	}
	if verify == nil {
		return fmt.Errorf("running core health verifier is unavailable")
	}
	if len(args) < 1 || args[0] != "health" || len(args) > 2 || len(args) == 2 && args[1] != "--stdin" {
		return fmt.Errorf("usage: omnidex health [--stdin]")
	}
	var err error
	if len(args) == 2 {
		err = verifyRunningCoreHealthDocument(stdin)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), localCoreHealthTimeout)
		defer cancel()
		err = verify(ctx, &http.Client{Timeout: localCoreHealthTimeout}, localCoreHealthURL)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "ok")
	return err
}

func verifyRunningCoreHealth(
	ctx context.Context,
	client *http.Client,
	endpoint string,
) error {
	if ctx == nil || client == nil {
		return fmt.Errorf("running core health verification requires context and HTTP client")
	}
	if endpoint != localCoreHealthURL {
		return fmt.Errorf("running core health verification requires the fixed loopback endpoint")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create running core health request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("reach running core health endpoint: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, localCoreHealthMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read running core health response: %w", err)
	}
	if len(body) > localCoreHealthMaxBytes {
		return fmt.Errorf("running core health response exceeds %d bytes", localCoreHealthMaxBytes)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("running core health returned HTTP %d", response.StatusCode)
	}
	return verifyRunningCoreHealthDocument(bytes.NewReader(body))
}

func verifyRunningCoreHealthDocument(
	document io.Reader,
) error {
	if document == nil {
		return fmt.Errorf("running core health document is unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(document, localCoreHealthMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read running core health document: %w", err)
	}
	if len(body) > localCoreHealthMaxBytes {
		return fmt.Errorf("running core health document exceeds %d bytes", localCoreHealthMaxBytes)
	}
	var health runningCoreHealth
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&health); err != nil {
		return fmt.Errorf("decode running core health response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("running core health response contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing running core health response: %w", err)
	}
	if health.Status != "ok" || !health.QueueEnabled || health.Time.IsZero() ||
		strings.TrimSpace(health.ListenAddr) == "" || health.Dependencies == nil {
		return fmt.Errorf("running core health is not fully operational")
	}
	postgres, exists := health.Dependencies["postgres"]
	if !exists || !postgres.Configured || !postgres.Reachable || postgres.Status != "ok" {
		return fmt.Errorf("running core dependency postgres is not configured, reachable, and healthy")
	}
	for name, dependency := range health.Dependencies {
		if dependency.Required || dependency.Configured {
			if dependency.Status != "ok" || !dependency.Configured || !dependency.Reachable {
				return fmt.Errorf("running core dependency %s is not configured, reachable, and healthy", name)
			}
		} else if dependency.Status != "not_configured" || dependency.Reachable {
			return fmt.Errorf("running core dependency %s has contradictory unconfigured state", name)
		}
	}
	return nil
}
