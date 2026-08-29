package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	directCodingReadinessBodyLimit = 1024
	directCodingReadinessTimeout   = 5 * time.Second
)

func probeDirectCodingDeploymentReadiness(
	parent context.Context,
	probeHost string,
	port uint16,
	readinessPath string,
) error {
	if parent == nil {
		return fmt.Errorf("deployment readiness observation requires a context")
	}
	if err := validateDirectCodingDeploymentHost("probe", probeHost); err != nil {
		return err
	}
	if port == 0 || readinessPath != directCodingDeploymentReadinessPath {
		return fmt.Errorf("deployment readiness endpoint is not registered")
	}
	endpoint := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(probeHost, strconv.Itoa(int(port))),
		Path:   readinessPath,
	}
	transport := &http.Transport{
		DisableCompression:     true,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: 16 << 10,
		ResponseHeaderTimeout:  directCodingReadinessTimeout,
		ExpectContinueTimeout:  time.Second,
		TLSHandshakeTimeout:    directCodingReadinessTimeout,
		IdleConnTimeout:        time.Second,
	}
	defer transport.CloseIdleConnections()
	return probeDirectCodingDeploymentReadinessWithClient(
		parent, newDirectCodingReadinessClient(transport), endpoint.String(),
	)
}

func newDirectCodingReadinessClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("deployment readiness redirects are forbidden")
		},
	}
}

func probeDirectCodingDeploymentReadinessWithClient(
	parent context.Context,
	client *http.Client,
	endpoint string,
) error {
	if parent == nil || client == nil {
		return fmt.Errorf("deployment readiness transport requires context and client authority")
	}
	ctx, cancel := context.WithTimeout(parent, directCodingReadinessTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("construct deployment readiness request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("execute deployment readiness request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, directCodingReadinessBodyLimit+1))
	if err != nil {
		return fmt.Errorf("read deployment readiness response: %w", err)
	}
	if len(body) > directCodingReadinessBodyLimit {
		return fmt.Errorf("deployment readiness response exceeds the %d-byte limit", directCodingReadinessBodyLimit)
	}
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("deployment readiness returned HTTP status %d", response.StatusCode)
	}
	if len(body) != 0 {
		return fmt.Errorf("deployment readiness 204 response contains a body")
	}
	return nil
}
