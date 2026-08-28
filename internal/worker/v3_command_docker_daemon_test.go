package worker

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateV3DockerDaemonAcceptsHealthyDefaultDaemon(t *testing.T) {
	socketPath, closeSocket := openV3DockerTestDaemon(
		t, http.StatusOK, `{"ID":"daemon-test","ServerVersion":"29.5.1","SecurityOptions":["name=seccomp,profile=builtin"]}`,
	)
	defer closeSocket()

	if err := validateV3DockerDaemon(context.Background(), socketPath); err != nil {
		t.Fatal(err)
	}
}

func TestValidateV3DockerDaemonRejectsInvalidInfo(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP failure", status: http.StatusServiceUnavailable, body: `{}`, want: "HTTP status 503"},
		{name: "oversized response", status: http.StatusOK, body: strings.Repeat("x", v3DockerInfoLimit+1), want: "exceeds"},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, want: "decode Docker daemon qualification"},
		{name: "missing identity", status: http.StatusOK, body: `{"ServerVersion":"29.5.1"}`, want: "identity"},
		{name: "unstable identity", status: http.StatusOK, body: `{"ID":" daemon-test ","ServerVersion":"29.5.1"}`, want: "identity"},
		{name: "missing version", status: http.StatusOK, body: `{"ID":"daemon-test"}`, want: "version"},
		{name: "unstable version", status: http.StatusOK, body: `{"ID":"daemon-test","ServerVersion":"29.5.1\nother"}`, want: "version"},
		{name: "missing security authority", status: http.StatusOK, body: `{"ID":"daemon-test","ServerVersion":"29.5.1"}`, want: "security options"},
		{name: "rootless daemon", status: http.StatusOK, body: `{"ID":"daemon-test","ServerVersion":"29.5.1","SecurityOptions":["name=rootless"]}`, want: "rootless"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			socketPath, closeSocket := openV3DockerTestDaemon(
				t, testCase.status, testCase.body,
			)
			defer closeSocket()

			err := validateV3DockerDaemon(context.Background(), socketPath)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("qualification error=%v want %q", err, testCase.want)
			}
		})
	}
}

func openV3DockerTestSocket(t *testing.T) (string, func()) {
	t.Helper()
	return openV3DockerTestDaemon(
		t, http.StatusOK, `{"ID":"daemon-test","ServerVersion":"29.5.1","SecurityOptions":["name=seccomp,profile=builtin"]}`,
	)
}

func openV3DockerTestDaemon(t *testing.T, status int, body string) (string, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/info" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(body))
	})}
	go func() { _ = server.Serve(listener) }()
	return path, func() {
		_ = server.Close()
		_ = listener.Close()
		_ = os.Remove(path)
	}
}
