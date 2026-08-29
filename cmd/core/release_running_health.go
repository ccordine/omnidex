package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	runningReleaseHealthURL     = "http://127.0.0.1:8090/healthz"
	runningReleaseHealthLimit   = 64 << 10
	runningReleaseHealthTimeout = 5 * time.Second
)

type runningReleaseHealthClient interface {
	Do(*http.Request) (*http.Response, error)
}

func verifyRunningReleaseHealthCommand(expectedCommit string) (string, error) {
	transport := &http.Transport{
		DisableCompression:     true,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: 16 << 10,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: runningReleaseHealthTimeout}
	ctx, cancel := context.WithTimeout(context.Background(), runningReleaseHealthTimeout)
	defer cancel()
	return verifyRunningReleaseHealth(ctx, client, runningReleaseHealthURL, expectedCommit)
}

func verifyRunningReleaseHealth(
	ctx context.Context,
	client runningReleaseHealthClient,
	endpoint string,
	expectedCommit string,
) (string, error) {
	if ctx == nil || client == nil {
		return "", fmt.Errorf("running release health requires context and HTTP client")
	}
	if _, err := verifyReleaseCommit(expectedCommit); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("construct running release health request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetch running release health: %w", err)
	}
	if response == nil || response.Body == nil {
		return "", fmt.Errorf("running release health returned no response body")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("running release health returned HTTP status %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, runningReleaseHealthLimit+1))
	if err != nil {
		return "", fmt.Errorf("read running release health: %w", err)
	}
	if len(payload) > runningReleaseHealthLimit {
		return "", fmt.Errorf("running release health exceeds %d bytes", runningReleaseHealthLimit)
	}
	return decodeRunningReleaseHealth(payload, expectedCommit)
}

func decodeRunningReleaseHealth(payload []byte, expectedCommit string) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var health struct {
		Release *struct {
			Commit string `json:"commit"`
		} `json:"release"`
	}
	if err := decoder.Decode(&health); err != nil {
		return "", fmt.Errorf("decode running release health: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", fmt.Errorf("running release health contains trailing JSON data")
	}
	if health.Release == nil || health.Release.Commit == "" {
		return "", fmt.Errorf("running release health omits release.commit")
	}
	if health.Release.Commit != expectedCommit {
		return "", fmt.Errorf(
			"running release health commit %s does not match expected commit %s",
			health.Release.Commit,
			expectedCommit,
		)
	}
	return health.Release.Commit, nil
}
