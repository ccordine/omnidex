package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	ollamaProviderBackend      = "ollama"
	ollamaAttestationBodyLimit = 4 * 1024 * 1024
)

type versionResponse struct {
	Version string `json:"version"`
}

type preloadRequest struct {
	Model     string      `json:"model"`
	Stream    bool        `json:"stream"`
	KeepAlive string      `json:"keep_alive"`
	Options   chatOptions `json:"options"`
}

type runningModelsResponse struct {
	Models []runningModel `json:"models"`
}

type runningModel struct {
	Name          string       `json:"name"`
	Model         string       `json:"model"`
	Size          int64        `json:"size"`
	Digest        string       `json:"digest"`
	Details       ModelDetails `json:"details"`
	ExpiresAt     time.Time    `json:"expires_at"`
	SizeVRAM      int64        `json:"size_vram"`
	ContextLength int          `json:"context_length"`
}

func (c *Client) AttestProviderIdentity(
	ctx context.Context,
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	if err := expected.Validate(); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	if expected.Backend != ollamaProviderBackend {
		return llm.ProviderIdentityAttestation{}, fmt.Errorf(
			"ollama cannot attest backend %q", expected.Backend,
		)
	}
	attestation, err := c.DiscoverProviderIdentity(ctx, llm.ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	})
	if err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	if err := attestation.ValidateFor(expected); err != nil {
		return llm.ProviderIdentityAttestation{}, fmt.Errorf(
			"live Ollama identity differs from frozen authority: %w", err,
		)
	}
	return attestation, nil
}

func (c *Client) DiscoverProviderIdentity(
	ctx context.Context,
	selection llm.ProviderIdentitySelection,
) (llm.ProviderIdentityAttestation, error) {
	if ctx == nil || c == nil || c.httpClient == nil || c.baseURL == "" {
		return llm.ProviderIdentityAttestation{}, fmt.Errorf("ollama identity discoverer is uninitialized")
	}
	if err := selection.Validate(); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	var version versionResponse
	if err := c.attestationJSON(ctx, http.MethodGet, "/api/version", nil, &version); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	var installed tagsResponse
	if err := c.attestationJSON(ctx, http.MethodGet, "/api/tags", nil, &installed); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	tag, err := exactInstalledModel(installed.Models, selection.Model)
	if err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	preload, err := json.Marshal(preloadRequest{
		Model: selection.Model, Stream: false, KeepAlive: "5m",
		Options: chatOptions{NumCtx: selection.NativeContextLimit},
	})
	if err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	var ignored map[string]any
	if err := c.attestationJSON(ctx, http.MethodPost, "/api/generate", preload, &ignored); err != nil {
		return llm.ProviderIdentityAttestation{}, fmt.Errorf("preload exact Ollama runner: %w", err)
	}
	var running runningModelsResponse
	if err := c.attestationJSON(ctx, http.MethodGet, "/api/ps", nil, &running); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	runner, err := exactRunningModel(running.Models, selection.Model)
	if err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	backendVersion := strings.TrimSpace(version.Version)
	expected := llm.ProviderIdentityExpectation{
		Backend: ollamaProviderBackend, BackendVersion: backendVersion,
		Model: selection.Model, Digest: tag.Digest,
		Quantization:       tag.Details.QuantizationLevel,
		NativeContextLimit: selection.NativeContextLimit,
	}
	if err := expected.Validate(); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	if err := requireOllamaModelIdentity(
		"running", runner.Digest, runner.Details.QuantizationLevel, expected,
	); err != nil {
		return llm.ProviderIdentityAttestation{}, err
	}
	if runner.ContextLength != selection.NativeContextLimit {
		return llm.ProviderIdentityAttestation{}, fmt.Errorf(
			"running Ollama context allocation %d differs from selected %d",
			runner.ContextLength, selection.NativeContextLimit,
		)
	}
	return llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
}

func (c *Client) attestationJSON(
	ctx context.Context,
	method string,
	endpoint string,
	body []byte,
	target any,
) error {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return c.wrapConnectivityError(err, endpoint)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, ollamaAttestationBodyLimit+1))
	if err != nil {
		return err
	}
	if len(raw) > ollamaAttestationBodyLimit {
		return fmt.Errorf("Ollama identity response %s exceeds %d bytes", endpoint, ollamaAttestationBodyLimit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Ollama identity request %s failed: status=%d body=%s", endpoint, response.StatusCode, raw)
	}
	if err := exactjson.ValidateObject(raw, target, "Ollama identity response "+endpoint); err != nil {
		return fmt.Errorf("Ollama identity response %s is inexact: %w", endpoint, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("Ollama identity response %s is invalid: %w", endpoint, err)
	}
	return nil
}

func exactInstalledModel(models []ModelInfo, name string) (ModelInfo, error) {
	var found []ModelInfo
	for _, model := range models {
		if model.Name == name && (model.Model == "" || model.Model == name) {
			found = append(found, model)
		}
	}
	if len(found) != 1 {
		return ModelInfo{}, fmt.Errorf("Ollama installed tag %q matched %d exact records", name, len(found))
	}
	return found[0], nil
}

func exactRunningModel(models []runningModel, name string) (runningModel, error) {
	var found []runningModel
	for _, model := range models {
		if model.Name == name && (model.Model == "" || model.Model == name) {
			found = append(found, model)
		}
	}
	if len(found) != 1 {
		return runningModel{}, fmt.Errorf("Ollama running model %q matched %d exact records", name, len(found))
	}
	return found[0], nil
}

func requireOllamaModelIdentity(
	source string,
	digest string,
	quantization string,
	expected llm.ProviderIdentityExpectation,
) error {
	if digest != expected.Digest {
		return fmt.Errorf("%s Ollama digest %q differs from frozen %q", source, digest, expected.Digest)
	}
	if quantization != expected.Quantization {
		return fmt.Errorf(
			"%s Ollama quantization %q differs from frozen %q",
			source, quantization, expected.Quantization,
		)
	}
	return nil
}
