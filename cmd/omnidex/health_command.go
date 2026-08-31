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
	Release      runningCoreHealthRelease         `json:"release"`
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

type runningCoreHealthRelease struct {
	Version          string `json:"version"`
	Codename         string `json:"codename"`
	Commit           string `json:"commit"`
	ReleaseScheme    string `json:"release_scheme"`
	NationalDexID    string `json:"national_dex_id"`
	NextMaturityName string `json:"next_maturity_name"`
	SourceSHA256     string `json:"source_sha256"`
	Date             string `json:"date"`
}

type runningCoreHealthVerifier func(
	context.Context,
	*http.Client,
	string,
	string,
) (string, error)

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
	stdinMode := len(args) == 4 && args[3] == "--stdin"
	if len(args) != 3 && !stdinMode || args[0] != "health" || args[1] != "--expect-commit" {
		return fmt.Errorf("usage: omnidex health --expect-commit COMMIT [--stdin]")
	}
	expectedCommit := args[2]
	if err := validateBuildCommit(expectedCommit); err != nil {
		return err
	}
	var commit string
	var err error
	if stdinMode {
		commit, err = verifyRunningCoreHealthDocument(stdin, expectedCommit)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), localCoreHealthTimeout)
		defer cancel()
		commit, err = verify(
			ctx,
			&http.Client{Timeout: localCoreHealthTimeout},
			localCoreHealthURL,
			expectedCommit,
		)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, commit)
	return err
}

func verifyRunningCoreHealth(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	expectedCommit string,
) (string, error) {
	if ctx == nil || client == nil {
		return "", fmt.Errorf("running core health verification requires context and HTTP client")
	}
	if endpoint != localCoreHealthURL {
		return "", fmt.Errorf("running core health verification requires the fixed loopback endpoint")
	}
	if err := validateBuildCommit(expectedCommit); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create running core health request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("reach running core health endpoint: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, localCoreHealthMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read running core health response: %w", err)
	}
	if len(body) > localCoreHealthMaxBytes {
		return "", fmt.Errorf("running core health response exceeds %d bytes", localCoreHealthMaxBytes)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("running core health returned HTTP %d", response.StatusCode)
	}
	return verifyRunningCoreHealthDocument(bytes.NewReader(body), expectedCommit)
}

func verifyRunningCoreHealthDocument(
	document io.Reader,
	expectedCommit string,
) (string, error) {
	if document == nil {
		return "", fmt.Errorf("running core health document is unavailable")
	}
	if err := validateBuildCommit(expectedCommit); err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(document, localCoreHealthMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read running core health document: %w", err)
	}
	if len(body) > localCoreHealthMaxBytes {
		return "", fmt.Errorf("running core health document exceeds %d bytes", localCoreHealthMaxBytes)
	}
	var health runningCoreHealth
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&health); err != nil {
		return "", fmt.Errorf("decode running core health response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", fmt.Errorf("running core health response contains multiple JSON values")
		}
		return "", fmt.Errorf("decode trailing running core health response: %w", err)
	}
	if health.Status != "ok" || !health.QueueEnabled || health.Time.IsZero() ||
		strings.TrimSpace(health.ListenAddr) == "" || health.Dependencies == nil {
		return "", fmt.Errorf("running core health is not fully operational")
	}
	for _, name := range []string{"postgres", "redis"} {
		dependency, exists := health.Dependencies[name]
		if !exists || dependency.Status != "ok" || !dependency.Configured || !dependency.Reachable {
			return "", fmt.Errorf(
				"running core dependency %s is not configured, reachable, and healthy",
				name,
			)
		}
	}
	if health.Release.Commit != expectedCommit {
		return "", fmt.Errorf(
			"running core health reports release commit %q, expected %q",
			health.Release.Commit,
			expectedCommit,
		)
	}
	return health.Release.Commit, nil
}

func validateBuildCommit(commit string) error {
	if len(commit) != 40 && len(commit) != 64 {
		return fmt.Errorf("expected commit must contain exactly 40 or 64 lowercase hexadecimal characters")
	}
	for _, character := range []byte(commit) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return fmt.Errorf("expected commit must contain exactly 40 or 64 lowercase hexadecimal characters")
		}
	}
	return nil
}
