package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	v3DockerInfoLimit          = 1 << 20
	v3DockerInfoAuthorityLimit = 256
	v3DockerInfoTimeout        = 3 * time.Second
)

func validateV3DockerDaemon(parent context.Context, socketPath string) error {
	if parent == nil {
		return fmt.Errorf("Docker daemon qualification requires a context")
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
		return fmt.Errorf("construct Docker daemon qualification request: %w", err)
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return fmt.Errorf("connect to required Docker daemon: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"Docker daemon qualification returned HTTP status %d", response.StatusCode,
		)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, v3DockerInfoLimit+1))
	if err != nil {
		return fmt.Errorf("read Docker daemon qualification: %w", err)
	}
	if len(body) > v3DockerInfoLimit {
		return fmt.Errorf("Docker daemon qualification exceeds the %d-byte limit", v3DockerInfoLimit)
	}
	if !utf8.Valid(body) {
		return fmt.Errorf("decode Docker daemon qualification: response is not valid UTF-8 JSON")
	}
	var info struct {
		ID              string   `json:"ID"`
		ServerVersion   string   `json:"ServerVersion"`
		SecurityOptions []string `json:"SecurityOptions"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("decode Docker daemon qualification: %w", err)
	}
	if !validV3DockerInfoAuthority(info.ID) {
		return fmt.Errorf("Docker daemon qualification returned an invalid stable identity")
	}
	if !validV3DockerInfoAuthority(info.ServerVersion) {
		return fmt.Errorf("Docker daemon qualification returned an invalid stable server version")
	}
	if info.SecurityOptions == nil {
		return fmt.Errorf("Docker daemon qualification omitted security options authority")
	}
	for _, option := range info.SecurityOptions {
		if !validV3DockerInfoAuthority(option) {
			return fmt.Errorf("Docker daemon qualification returned an invalid security option")
		}
		if option == "name=rootless" || strings.HasPrefix(option, "name=rootless,") {
			return fmt.Errorf("Docker daemon qualification rejected rootless execution authority")
		}
	}
	return nil
}

func validV3DockerInfoAuthority(value string) bool {
	if value == "" || len(value) > v3DockerInfoAuthorityLimit ||
		value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
