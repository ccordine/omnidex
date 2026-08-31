package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes int64 = 4 * 1024 * 1024

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (err *HTTPError) Error() string {
	if err == nil {
		return "HTTP request failed"
	}
	if err.Message != "" {
		return err.Message
	}
	return fmt.Sprintf("HTTP request failed with status %d", err.StatusCode)
}

func IsHTTPStatus(err error, status int) bool {
	var responseErr *HTTPError
	return errors.As(err, &responseErr) && responseErr.StatusCode == status
}

func New(baseURL string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("CORE_URL must be one absolute HTTP or HTTPS URL")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("client request timeout must be positive")
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (client *Client) doJSON(
	ctx context.Context,
	method string,
	requestPath string,
	payload any,
	destination any,
	expectedStatus int,
) error {
	return client.doJSONBounded(
		ctx,
		method,
		requestPath,
		payload,
		destination,
		expectedStatus,
		maxResponseBytes,
	)
}

func (client *Client) doJSONBounded(
	ctx context.Context,
	method string,
	requestPath string,
	payload any,
	destination any,
	expectedStatus int,
	responseLimit int64,
) error {
	if client == nil || client.httpClient == nil {
		return fmt.Errorf("Omnidex client is unavailable")
	}
	if responseLimit < 1 {
		return fmt.Errorf("Omnidex response byte limit must be positive")
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode Omnidex request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+requestPath, body)
	if err != nil {
		return fmt.Errorf("create Omnidex request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send Omnidex request: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return fmt.Errorf("read Omnidex response: %w", err)
	}
	if int64(len(data)) > responseLimit {
		return fmt.Errorf("Omnidex response exceeds %d bytes", responseLimit)
	}
	if response.StatusCode != expectedStatus {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &body)
		message := strings.TrimSpace(body.Error)
		if message == "" {
			message = fmt.Sprintf(
				"Omnidex request returned status %d, expected %d",
				response.StatusCode,
				expectedStatus,
			)
		}
		return &HTTPError{StatusCode: response.StatusCode, Message: message}
	}
	if destination == nil {
		if strings.TrimSpace(string(data)) != "" {
			return fmt.Errorf("Omnidex response unexpectedly contains a JSON body")
		}
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode Omnidex response: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}
