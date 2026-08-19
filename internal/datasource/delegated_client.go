package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

type DelegatedClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewDelegatedClient(baseURL, token string, httpClient *http.Client) (*DelegatedClient, error) {
	if err := ValidateDelegatedBaseURL(baseURL); err != nil {
		return nil, err
	}
	if token == "" || token != strings.TrimSpace(token) || len(token) < 24 || len(token) > 4096 ||
		strings.ContainsAny(token, "\r\n\x00") {
		return nil, fmt.Errorf("delegated database authority token must contain 24..4096 exact bytes")
	}
	if httpClient == nil || httpClient.Timeout <= 0 {
		return nil, fmt.Errorf("delegated database authority requires a bounded HTTP client")
	}
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("delegated database authority redirects are forbidden")
	}
	return &DelegatedClient{baseURL: baseURL, token: token, httpClient: &client}, nil
}

func ValidateDelegatedBaseURL(baseURL string) error {
	if baseURL == "" || baseURL != strings.TrimSpace(baseURL) {
		return fmt.Errorf("delegated database authority URL must be exact nonblank text")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("delegated database authority URL must be one HTTP(S) origin or base path")
	}
	if parsed.Path != "" && (path.Clean(parsed.Path) != parsed.Path || strings.HasSuffix(parsed.Path, "/")) {
		return fmt.Errorf("delegated database authority URL path must be canonical without a trailing slash")
	}
	return nil
}

func (client *DelegatedClient) FetchSchema(
	ctx context.Context,
	sourceID, sourceName, authorityID string,
) (SchemaSnapshot, error) {
	request := DelegatedSchemaRequest{
		Schema: DelegatedSchemaRequestV1, SourceID: sourceID, AuthorityID: authorityID,
	}
	if err := request.Validate(); err != nil {
		return SchemaSnapshot{}, err
	}
	var response DelegatedSchemaResponse
	if err := client.post(ctx, DelegatedSchemaPath, request, &response); err != nil {
		return SchemaSnapshot{}, err
	}
	return response.Snapshot(sourceID, sourceName)
}

func (client *DelegatedClient) Execute(
	ctx context.Context,
	authorityID string,
	snapshot SchemaSnapshot,
	plan RelationalQueryPlan,
	limits ExecutionLimits,
) (EvidenceResult, error) {
	transportLimits, err := NewDelegatedExecutionLimits(limits)
	if err != nil {
		return EvidenceResult{}, err
	}
	request := DelegatedEvidenceRequest{
		Schema: DelegatedEvidenceRequestV1, AuthorityID: authorityID,
		Snapshot: snapshot, Plan: plan, Limits: transportLimits,
	}
	if _, err := request.Validate(snapshot); err != nil {
		return EvidenceResult{}, err
	}
	var response DelegatedEvidenceResponse
	if err := client.post(ctx, DelegatedEvidencePath, request, &response); err != nil {
		return EvidenceResult{}, err
	}
	if err := response.Validate(snapshot, plan, limits); err != nil {
		return EvidenceResult{}, err
	}
	return response.Evidence, nil
}

func (client *DelegatedClient) post(ctx context.Context, endpoint string, input, output any) error {
	if client == nil || client.httpClient == nil || ctx == nil {
		return fmt.Errorf("delegated database request requires client and context")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode delegated database request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create delegated database request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("execute delegated database request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxDelegatedResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read delegated database response: %w", err)
	}
	if len(raw) > MaxDelegatedResponseBytes {
		return fmt.Errorf("delegated database response exceeds %d bytes", MaxDelegatedResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return delegatedHostStatusError(response.StatusCode, raw)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("delegated database host returned a non-JSON response")
	}
	if err := decodeExactDelegatedJSON(raw, output); err != nil {
		return fmt.Errorf("decode delegated database response: %w", err)
	}
	return nil
}

func delegatedHostStatusError(status int, raw []byte) error {
	var response DelegatedErrorResponse
	if err := decodeExactDelegatedJSON(raw, &response); err != nil || response.Validate() != nil {
		return fmt.Errorf("delegated database host failed with HTTP %d", status)
	}
	return fmt.Errorf("delegated database host failed with HTTP %d code %s", status, response.ErrorCode)
}

func decodeExactDelegatedJSON(raw []byte, destination any) error {
	if len(raw) == 0 || !utf8.Valid(raw) {
		return fmt.Errorf("response must contain valid UTF-8 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("response must contain exactly one JSON value")
	}
	return nil
}
