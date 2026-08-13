package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "HTTP request failed"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("HTTP request failed with status %d", e.StatusCode)
}

func IsHTTPStatus(err error, status int) bool {
	var responseError *HTTPError
	return errors.As(err, &responseError) && responseError.StatusCode == status
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := ""
		var errorBody struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &errorBody) == nil {
			message = strings.TrimSpace(errorBody.Error)
		}
		if message == "" {
			message = fmt.Sprintf(
				"request failed: status=%d body=%s",
				resp.StatusCode,
				strings.TrimSpace(string(data)),
			)
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: message}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}
