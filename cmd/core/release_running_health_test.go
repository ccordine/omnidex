package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/version"
)

type runningReleaseHealthClientFunc func(*http.Request) (*http.Response, error)

func (function runningReleaseHealthClientFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestVerifyRunningReleaseHealthReturnsExactLiveCommit(t *testing.T) {
	commit := setRunningReleaseHealthTestCommit(t, "a", 40)
	client := runningReleaseHealthClientFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != runningReleaseHealthURL {
			t.Fatalf("running health request = %s %s", request.Method, request.URL)
		}
		return runningReleaseHealthResponse(
			http.StatusOK,
			"{\n  \"status\": \"ok\",\n  \"release\": {\"commit\": \""+commit+"\"}\n}\n",
		), nil
	})
	got, err := verifyRunningReleaseHealth(
		context.Background(), client, runningReleaseHealthURL, commit,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != commit {
		t.Fatalf("running health commit = %q, want %q", got, commit)
	}
}

func TestVerifyRunningReleaseHealthRejectsStaleCommit(t *testing.T) {
	commit := setRunningReleaseHealthTestCommit(t, "b", 40)
	stale := strings.Repeat("c", 40)
	client := runningReleaseHealthClientFunc(func(*http.Request) (*http.Response, error) {
		return runningReleaseHealthResponse(
			http.StatusOK,
			`{"release":{"commit":"`+stale+`"}}`,
		), nil
	})
	_, err := verifyRunningReleaseHealth(
		context.Background(), client, runningReleaseHealthURL, commit,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match expected commit") {
		t.Fatalf("stale running health error = %v", err)
	}
}

func TestVerifyRunningReleaseHealthRejectsOversizedPayload(t *testing.T) {
	commit := setRunningReleaseHealthTestCommit(t, "d", 64)
	client := runningReleaseHealthClientFunc(func(*http.Request) (*http.Response, error) {
		return runningReleaseHealthResponse(
			http.StatusOK,
			strings.Repeat(" ", runningReleaseHealthLimit+1),
		), nil
	})
	_, err := verifyRunningReleaseHealth(
		context.Background(), client, runningReleaseHealthURL, commit,
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized running health error = %v", err)
	}
}

func TestDecodeRunningReleaseHealthRejectsTrailingJSON(t *testing.T) {
	commit := strings.Repeat("e", 40)
	_, err := decodeRunningReleaseHealth(
		[]byte(`{"release":{"commit":"`+commit+`"}} {}`),
		commit,
	)
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing running health error = %v", err)
	}
}

func setRunningReleaseHealthTestCommit(t *testing.T, character string, length int) string {
	t.Helper()
	original := version.Commit
	t.Cleanup(func() { version.Commit = original })
	commit := strings.Repeat(character, length)
	version.Commit = commit
	return commit
}

func runningReleaseHealthResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
