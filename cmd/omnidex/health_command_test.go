package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testBuildCommit = "0123456789abcdef0123456789abcdef01234567"

type healthRoundTripper func(*http.Request) (*http.Response, error)

func (transport healthRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestVerifyRunningCoreHealth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		status       int
		dependencies string
		body         string
		wantErr      string
	}{
		{
			name:         "fully operational exact release",
			status:       http.StatusOK,
			dependencies: healthyDependenciesJSON,
		},
		{
			name:         "degraded aggregate state",
			status:       http.StatusOK,
			dependencies: healthyDependenciesJSON,
			body:         healthResponseJSON("degraded", testBuildCommit, healthyDependenciesJSON, ""),
			wantErr:      "not fully operational",
		},
		{
			name:         "missing redis dependency",
			status:       http.StatusOK,
			dependencies: `{"postgres":{"configured":true,"reachable":true,"required":true,"status":"ok"}}`,
			wantErr:      "dependency redis",
		},
		{
			name:         "redis is not configured",
			status:       http.StatusOK,
			dependencies: `{"postgres":{"configured":true,"reachable":true,"required":true,"status":"ok"},"redis":{"configured":false,"reachable":false,"required":false,"status":"not_configured"}}`,
			wantErr:      "dependency redis",
		},
		{
			name:         "redis is unreachable",
			status:       http.StatusOK,
			dependencies: `{"postgres":{"configured":true,"reachable":true,"required":true,"status":"ok"},"redis":{"configured":true,"reachable":false,"required":false,"status":"error"}}`,
			wantErr:      "dependency redis",
		},
		{
			name:         "different running release",
			status:       http.StatusOK,
			dependencies: healthyDependenciesJSON,
			body:         healthResponseJSON("ok", strings.Repeat("a", 40), healthyDependenciesJSON, ""),
			wantErr:      "reports release commit",
		},
		{
			name:         "unknown response authority",
			status:       http.StatusOK,
			dependencies: healthyDependenciesJSON,
			body:         healthResponseJSON("ok", testBuildCommit, healthyDependenciesJSON, `,"unexpected":true`),
			wantErr:      "unknown field",
		},
		{
			name:         "multiple JSON values",
			status:       http.StatusOK,
			dependencies: healthyDependenciesJSON,
			body:         healthResponseJSON("ok", testBuildCommit, healthyDependenciesJSON, "") + `{}`,
			wantErr:      "multiple JSON values",
		},
		{
			name:    "non-success response",
			status:  http.StatusServiceUnavailable,
			body:    `{}`,
			wantErr: "HTTP 503",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := test.body
			if body == "" {
				body = healthResponseJSON("ok", testBuildCommit, test.dependencies, "")
			}
			client := &http.Client{Transport: healthRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != localCoreHealthURL {
					t.Fatalf("health request URL = %q", request.URL.String())
				}
				return &http.Response{
					StatusCode: test.status,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})}
			commit, err := verifyRunningCoreHealth(
				context.Background(),
				client,
				localCoreHealthURL,
				testBuildCommit,
			)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("verify health: %v", err)
				}
				if commit != testBuildCommit {
					t.Fatalf("commit = %q", commit)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestRunHealthCommand(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	called := false
	err := runCommandWithVerifier(
		[]string{"health", "--expect-commit", testBuildCommit},
		nil,
		&output,
		func(
			_ context.Context,
			_ *http.Client,
			endpoint string,
			expectedCommit string,
		) (string, error) {
			called = true
			if endpoint != localCoreHealthURL || expectedCommit != testBuildCommit {
				t.Fatalf("verifier authority = %q, %q", endpoint, expectedCommit)
			}
			return expectedCommit, nil
		},
	)
	if err != nil {
		t.Fatalf("run health command: %v", err)
	}
	if !called || output.String() != testBuildCommit+"\n" {
		t.Fatalf("called = %t, output = %q", called, output.String())
	}
}

func TestRunHealthCommandReadsDocumentFromStdin(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := runCommandWithVerifier(
		[]string{"health", "--expect-commit", testBuildCommit, "--stdin"},
		strings.NewReader(healthResponseJSON("ok", testBuildCommit, healthyDependenciesJSON, "")),
		&output,
		func(context.Context, *http.Client, string, string) (string, error) {
			t.Fatal("HTTP verifier was called for stdin health document")
			return "", nil
		},
	)
	if err != nil {
		t.Fatalf("run stdin health command: %v", err)
	}
	if output.String() != testBuildCommit+"\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunHealthCommandRejectsInvalidInterface(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"health"},
		{"health", "--expect-commit", "not-a-commit"},
		{"health", "--expect-commit", testBuildCommit, "--stdin"},
		{"version", "--json"},
	} {
		if err := runCommandWithVerifier(args, nil, io.Discard, verifyRunningCoreHealth); err == nil {
			t.Fatalf("args %q unexpectedly accepted", args)
		}
	}
}

const healthyDependenciesJSON = `{"postgres":{"configured":true,"reachable":true,"required":true,"status":"ok"},"redis":{"configured":true,"reachable":true,"required":false,"status":"ok"}}`

func healthResponseJSON(status, commit, dependencies, suffix string) string {
	return `{"dependencies":` + dependencies + `,` +
		`"listen_addr":":8090","queue_enabled":true,` +
		`"release":{"codename":"Charmeleon","commit":"` + commit + `",` +
		`"date":"","national_dex_id":"5","next_maturity_name":"Charizard",` +
		`"release_scheme":"pride-national-dex","source_sha256":"","version":"v0.5.0"},` +
		`"status":"` + status + `","time":"2026-08-31T12:00:00Z"` + suffix + `}`
}
