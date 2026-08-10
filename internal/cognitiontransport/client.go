package cognitiontransport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

var _ cognition.Environment = (*Client)(nil)
var _ cognitionruntime.CompletionEvaluator = (*Client)(nil)

func NewClient(baseURL, token string, client *http.Client) (*Client, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" ||
		token == "" || token != strings.TrimSpace(token) || len(token) > 4096 || client == nil {
		return nil, fmt.Errorf("cognition environment client configuration is invalid")
	}
	return &Client{baseURL: baseURL, token: token, http: client}, nil
}

func (client *Client) Start(ctx context.Context, scenario cognition.ScenarioRef) (cognition.Transition, error) {
	if err := scenario.Validate(); err != nil {
		return cognition.Transition{}, err
	}
	response, err := client.call(ctx, startPath, startRequest{Protocol: ProtocolVersionV1, Scenario: scenario})
	if err != nil {
		return cognition.Transition{}, err
	}
	if response.Transition == nil {
		return cognition.Transition{}, ErrInvalidWire
	}
	if err := response.Transition.ValidateStart(); err != nil {
		return cognition.Transition{}, fmt.Errorf("%w: %v", ErrInvalidWire, err)
	}
	return response.Transition.Clone(), nil
}

func (client *Client) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	if err := episode.Validate(); err != nil {
		return cognition.Transition{}, err
	}
	if err := expected.Validate(); err != nil {
		return cognition.Transition{}, err
	}
	response, err := client.call(ctx, applyPath, applyRequest{
		Protocol: ProtocolVersionV1, Episode: episode, Expected: expected, Action: action,
	})
	if err != nil {
		return cognition.Transition{}, err
	}
	if response.Failure != nil {
		if err := response.Failure.Validate(action, expected); err != nil {
			return cognition.Transition{}, fmt.Errorf("%w: %v", ErrInvalidWire, err)
		}
		return cognition.Transition{}, response.Failure.Clone()
	}
	if response.Transition == nil {
		return cognition.Transition{}, ErrInvalidWire
	}
	if err := response.Transition.ValidateApply(episode, expected, action); err != nil {
		return cognition.Transition{}, fmt.Errorf("%w: %v", ErrInvalidWire, err)
	}
	return response.Transition.Clone(), nil
}

func (client *Client) Evaluate(
	ctx context.Context,
	request cognitionruntime.CompletionRequest,
) (cognition.CompletionResult, error) {
	if err := validateCompletionRequest(request); err != nil {
		return cognition.CompletionResult{}, err
	}
	response, err := client.call(ctx, evaluatePath, evaluateRequest{
		Protocol: ProtocolVersionV1, Request: request,
	})
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if response.Completion == nil {
		return cognition.CompletionResult{}, ErrInvalidWire
	}
	if err := response.Completion.ValidateFor(
		request.Obligation, request.Revision, request.EvidenceRefs,
	); err != nil {
		return cognition.CompletionResult{}, fmt.Errorf("%w: %v", ErrInvalidWire, err)
	}
	return response.Completion.Clone(), nil
}

func (client *Client) call(ctx context.Context, path string, input any) (wireResponse, error) {
	if client == nil || client.http == nil || ctx == nil {
		return wireResponse{}, fmt.Errorf("cognition environment client is unavailable")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return wireResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return wireResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	result, err := client.http.Do(request)
	if err != nil {
		return wireResponse{}, err
	}
	defer result.Body.Close()
	limited := io.LimitReader(result.Body, maxRequestBytes+1)
	responseRaw, err := io.ReadAll(limited)
	if err != nil || len(responseRaw) > maxRequestBytes {
		return wireResponse{}, fmt.Errorf("%w: response exceeds transport limit", ErrInvalidWire)
	}
	if err := cognition.ValidateExactJSONObject(responseRaw, wireResponse{}, "cognition transport response"); err != nil {
		return wireResponse{}, fmt.Errorf("%w: %v", ErrInvalidWire, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(responseRaw))
	decoder.DisallowUnknownFields()
	var response wireResponse
	if err := decoder.Decode(&response); err != nil {
		return wireResponse{}, fmt.Errorf("%w: %v", ErrInvalidWire, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || response.Protocol != ProtocolVersionV1 {
		return wireResponse{}, ErrInvalidWire
	}
	count := 0
	if response.Transition != nil {
		count++
	}
	if response.Failure != nil {
		count++
	}
	if response.Completion != nil {
		count++
	}
	if response.Error != nil {
		count++
	}
	if count != 1 {
		return wireResponse{}, ErrInvalidWire
	}
	if response.Transition != nil && result.StatusCode != http.StatusOK ||
		response.Failure != nil && result.StatusCode != http.StatusConflict ||
		response.Completion != nil && result.StatusCode != http.StatusOK ||
		response.Error != nil && result.StatusCode < http.StatusBadRequest {
		return wireResponse{}, ErrInvalidWire
	}
	if response.Error != nil {
		if !validWireText(response.Error.Code, 128) || !validWireText(response.Error.Message, 4096) {
			return wireResponse{}, ErrInvalidWire
		}
		return wireResponse{}, RemoteError{Code: response.Error.Code, Message: response.Error.Message}
	}
	return response, nil
}

func validWireText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) &&
		!strings.ContainsRune(value, '\x00') && len(value) <= maximum
}
