package omnidex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type APIError struct {
	Status  int
	Message string
}

func (err *APIError) Error() string {
	return fmt.Sprintf("Omnidex integration API failed with HTTP %d: %s", err.Status, err.Message)
}

func NewClient(baseURL, token string) (*Client, error) {
	return NewClientWithHTTPClient(baseURL, token, &http.Client{Timeout: 30 * time.Second})
}

func NewClientWithHTTPClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	if err := validateClientConfiguration(baseURL, token); err != nil {
		return nil, err
	}
	if httpClient == nil || httpClient.Timeout <= 0 {
		return nil, fmt.Errorf("Omnidex SDK requires a bounded HTTP client")
	}
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("Omnidex integration API redirects are forbidden")
	}
	return &Client{baseURL: baseURL, token: token, httpClient: &client}, nil
}

func (client *Client) RegisterDirectDataSource(
	ctx context.Context,
	input DirectDataSourceInput,
) (DataSource, error) {
	if err := validateDirectDataSource(input); err != nil {
		return DataSource{}, err
	}
	payload := dataSourceRequest{
		Name: input.Name, Driver: "postgres", ExecutionMode: "direct",
		Host: input.Host, Port: input.Port, DatabaseName: input.DatabaseName,
		Username: input.Username, Password: input.Password, SSLMode: input.SSLMode,
		UseDSN: input.UseDSN, DSN: input.DSN,
	}
	return client.registerDataSource(ctx, payload)
}

func (client *Client) RegisterDelegatedDataSource(
	ctx context.Context,
	input DelegatedDataSourceInput,
) (DataSource, error) {
	if err := validateDelegatedDataSource(input); err != nil {
		return DataSource{}, err
	}
	payload := dataSourceRequest{
		Name: input.Name, Driver: "postgres", ExecutionMode: "delegated",
		AuthorityURL: input.AuthorityURL, CredentialEnv: input.CredentialEnv,
	}
	return client.registerDataSource(ctx, payload)
}

func (client *Client) registerDataSource(ctx context.Context, input dataSourceRequest) (DataSource, error) {
	var response struct {
		Source DataSource `json:"source"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/integrations/data-sources", nil, input, http.StatusCreated, &response); err != nil {
		return DataSource{}, err
	}
	if response.Source.ID == "" || !response.Source.ReadOnly || response.Source.Driver != "postgres" {
		return DataSource{}, fmt.Errorf("Omnidex returned an invalid data-source authority")
	}
	return response.Source, nil
}

func (client *Client) CreateChannel(ctx context.Context, input CreateChannelInput) (Channel, error) {
	if err := validateCreateChannel(input); err != nil {
		return Channel{}, err
	}
	payload := map[string]any{
		"id": input.ID, "name": input.Name, "tags": input.Tags,
		"workspace_root": input.WorkspaceRoot, "data_source_id": input.DataSourceID,
		"mode": "assistant",
	}
	var response struct {
		Channel Channel `json:"channel"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/integrations/channels", nil, payload, http.StatusCreated, &response); err != nil {
		return Channel{}, err
	}
	if response.Channel.ID != input.ID || response.Channel.DataSourceID != input.DataSourceID ||
		response.Channel.Mode != "assistant" {
		return Channel{}, fmt.Errorf("Omnidex returned a channel outside the requested authority")
	}
	return response.Channel, nil
}

func (client *Client) GetChannel(ctx context.Context, channelID string) (Channel, error) {
	if err := validateCanonicalID("channel ID", channelID, 96); err != nil {
		return Channel{}, err
	}
	var response struct {
		Channel Channel `json:"channel"`
	}
	path := "/v1/integrations/channels/" + channelID
	if err := client.request(ctx, http.MethodGet, path, nil, nil, http.StatusOK, &response); err != nil {
		return Channel{}, err
	}
	if response.Channel.ID != channelID || response.Channel.Mode != "assistant" {
		return Channel{}, fmt.Errorf("Omnidex returned a channel outside the requested authority")
	}
	return response.Channel, nil
}

func (client *Client) SendMessage(
	ctx context.Context,
	channelID string,
	input SendMessageInput,
) (SendMessageResult, error) {
	if err := validateCanonicalID("channel ID", channelID, 96); err != nil {
		return SendMessageResult{}, err
	}
	if err := validatePrompt(input.Prompt); err != nil {
		return SendMessageResult{}, err
	}
	payload := map[string]any{"prompt": input.Prompt}
	if input.DelegatedDataAuthorityID != "" {
		if err := ValidateDelegatedAuthorityID(input.DelegatedDataAuthorityID); err != nil {
			return SendMessageResult{}, err
		}
		payload["delegated_data_authority_id"] = input.DelegatedDataAuthorityID
	}
	var response SendMessageResult
	path := "/v1/integrations/channels/" + channelID + "/messages"
	if err := client.request(ctx, http.MethodPost, path, nil, payload, http.StatusAccepted, &response); err != nil {
		return SendMessageResult{}, err
	}
	if response.Channel.ID != channelID || response.UserMessage.ChannelID != channelID ||
		response.UserMessage.Content != input.Prompt || response.Job.ID < 1 {
		return SendMessageResult{}, fmt.Errorf("Omnidex returned a message outside the requested authority")
	}
	return response, nil
}

func (client *Client) ListMessages(
	ctx context.Context,
	channelID string,
	limit int,
	beforeID *int64,
) (MessagePage, error) {
	if err := validateCanonicalID("channel ID", channelID, 96); err != nil {
		return MessagePage{}, err
	}
	if limit < 1 || limit > 200 || beforeID != nil && *beforeID < 1 {
		return MessagePage{}, fmt.Errorf("message page bounds are invalid")
	}
	query := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if beforeID != nil {
		query.Set("before_id", strconv.FormatInt(*beforeID, 10))
	}
	var response MessagePage
	path := "/v1/integrations/channels/" + channelID + "/messages"
	if err := client.request(ctx, http.MethodGet, path, query, nil, http.StatusOK, &response); err != nil {
		return MessagePage{}, err
	}
	if response.ChannelID != channelID || response.HasMore != (response.NextBeforeID != nil) {
		return MessagePage{}, fmt.Errorf("Omnidex returned contradictory message-page authority")
	}
	return response, nil
}

func (client *Client) GetJob(ctx context.Context, jobID int64) (JobDetails, error) {
	if jobID < 1 {
		return JobDetails{}, fmt.Errorf("job ID must be positive")
	}
	var response JobDetails
	path := "/v1/integrations/jobs/" + strconv.FormatInt(jobID, 10)
	if err := client.request(ctx, http.MethodGet, path, nil, nil, http.StatusOK, &response); err != nil {
		return JobDetails{}, err
	}
	if response.Job.ID != jobID {
		return JobDetails{}, fmt.Errorf("Omnidex returned a different job authority")
	}
	return response, nil
}

func (client *Client) request(
	ctx context.Context,
	method, endpoint string,
	query url.Values,
	input any,
	expectedStatus int,
	output any,
) error {
	if client == nil || client.httpClient == nil || ctx == nil {
		return fmt.Errorf("Omnidex request requires a client and context")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode Omnidex request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	requestURL := client.baseURL + endpoint
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("create Omnidex request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("execute Omnidex request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read Omnidex response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("Omnidex response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode != expectedStatus {
		return decodeAPIError(response.StatusCode, raw)
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		return fmt.Errorf("Omnidex returned a non-JSON response")
	}
	return decodeExactJSON(raw, output)
}

func decodeAPIError(status int, raw []byte) error {
	var response struct {
		Error string `json:"error"`
	}
	if err := decodeExactJSON(raw, &response); err != nil || strings.TrimSpace(response.Error) == "" {
		return &APIError{Status: status, Message: "invalid error envelope"}
	}
	return &APIError{Status: status, Message: response.Error}
}

func decodeExactJSON(raw []byte, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode Omnidex response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("Omnidex response must contain exactly one JSON value")
	}
	return nil
}

type dataSourceRequest struct {
	Name          string `json:"name"`
	Driver        string `json:"driver"`
	ExecutionMode string `json:"execution_mode"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	DatabaseName  string `json:"database_name"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SSLMode       string `json:"ssl_mode"`
	UseDSN        bool   `json:"use_dsn"`
	DSN           string `json:"dsn"`
	AuthorityURL  string `json:"authority_url"`
	CredentialEnv string `json:"credential_env"`
}
