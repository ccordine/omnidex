package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	v3DockerInfoLimit   = 1 << 20
	v3DockerInfoTimeout = 3 * time.Second
)

func validateV3RootlessDockerDaemon(parent context.Context, socketPath string) error {
	if parent == nil {
		return fmt.Errorf("Docker rootless qualification requires a context")
	}
	ctx, cancel := context.WithTimeout(parent, v3DockerInfoTimeout)
	defer cancel()
	transport := &http.Transport{
		DisableCompression: true,
		DisableKeepAlives:  true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker.local/info", nil)
	if err != nil {
		return fmt.Errorf("construct rootless Docker qualification request: %w", err)
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return fmt.Errorf("connect to required rootless Docker daemon: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"rootless Docker qualification returned HTTP status %d", response.StatusCode,
		)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, v3DockerInfoLimit+1))
	if err != nil {
		return fmt.Errorf("read rootless Docker qualification: %w", err)
	}
	if len(body) > v3DockerInfoLimit {
		return fmt.Errorf("rootless Docker qualification exceeds the %d-byte limit", v3DockerInfoLimit)
	}
	var info struct {
		SecurityOptions []string `json:"SecurityOptions"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("decode rootless Docker qualification: %w", err)
	}
	for _, option := range info.SecurityOptions {
		if option == "name=rootless" {
			return nil
		}
	}
	return fmt.Errorf("Docker daemon rejected: security options do not declare rootless mode")
}
