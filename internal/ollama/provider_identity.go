package ollama

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

const ollamaProviderBackend = "ollama"

func (c *Client) ObserveProviderIdentity(
	ctx context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	if err := request.Validate(); err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	expected := request.Expectation
	if expected.Backend != ollamaProviderBackend {
		return llm.ObservedProviderIdentity{}, fmt.Errorf(
			"ollama cannot attest backend %q", expected.Backend,
		)
	}
	observed, err := c.discoverProviderIdentity(ctx, llm.ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}, request.ChallengeSHA256)
	if err != nil {
		return observed, err
	}
	if err := observed.ValidateFor(request); err != nil {
		return observed, fmt.Errorf(
			"live Ollama identity differs from frozen authority: %w", err,
		)
	}
	return observed, nil
}

func (c *Client) DiscoverProviderIdentityEvidence(
	ctx context.Context,
	selection llm.ProviderIdentitySelection,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	return c.discoverProviderIdentity(ctx, selection, challenge)
}

func (c *Client) discoverProviderIdentity(
	ctx context.Context,
	selection llm.ProviderIdentitySelection,
	challengeSHA256 string,
) (llm.ObservedProviderIdentity, error) {
	if ctx == nil || c == nil || c.httpClient == nil || c.baseURL == "" {
		return llm.ObservedProviderIdentity{}, fmt.Errorf("ollama identity discoverer is uninitialized")
	}
	if err := selection.Validate(); err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	showRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	specs := []providerIdentityOperationSpec{
		{llm.ProviderIdentityVersion, http.MethodGet, "/api/version", nil},
		{llm.ProviderIdentityInstalled, http.MethodGet, "/api/tags", nil},
		{llm.ProviderIdentityTokenizer, http.MethodPost, "/api/show", showRequest},
		{llm.ProviderIdentityPreload, http.MethodPost, "/api/generate", preloadRequest},
		{llm.ProviderIdentityRunner, http.MethodGet, "/api/ps", nil},
	}
	operations := make([]llm.ProviderIdentityOperationEvidence, 0, len(specs))
	for index, spec := range specs {
		operation, operationErr := c.observeProviderIdentityOperation(ctx, spec)
		if operationErr != nil {
			if operation.Operation == "" {
				return llm.ObservedProviderIdentity{}, operationErr
			}
			operations = append(operations, operation)
			for _, pending := range specs[index+1:] {
				operations = append(operations, pending.notDispatched())
			}
			evidence, evidenceErr := llm.NewProviderIdentityEvidence(operations)
			if evidenceErr != nil {
				return llm.ObservedProviderIdentity{}, evidenceErr
			}
			return llm.ObservedProviderIdentity{Evidence: evidence}, operationErr
		}
		operations = append(operations, operation)
	}
	evidence, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		return llm.ObservedProviderIdentity{Evidence: evidence}, err
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC(), attestation, evidence, challengeSHA256,
	)
	if err != nil {
		return llm.ObservedProviderIdentity{Evidence: evidence}, err
	}
	return observed, nil
}

type providerIdentityOperationSpec struct {
	operation        llm.ProviderIdentityOperation
	method, endpoint string
	request          []byte
}

func (spec providerIdentityOperationSpec) notDispatched() llm.ProviderIdentityOperationEvidence {
	value, err := llm.NewProviderIdentityOperationEvidence(
		spec.operation, spec.method, spec.endpoint, false, spec.request, 0,
		llm.ProviderIdentityNotDispatched, false, 0, "", false, nil,
	)
	if err != nil {
		panic(fmt.Sprintf("construct pending provider identity operation: %v", err))
	}
	return value
}

func (c *Client) observeProviderIdentityOperation(
	ctx context.Context,
	spec providerIdentityOperationSpec,
) (llm.ProviderIdentityOperationEvidence, error) {
	request, err := http.NewRequestWithContext(
		ctx, spec.method, c.baseURL+spec.endpoint, bytes.NewReader(spec.request),
	)
	if err != nil {
		return llm.ProviderIdentityOperationEvidence{}, err
	}
	if len(spec.request) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.doExactProviderRequest(request)
	if err != nil {
		operation, evidenceErr := spec.evidence(
			0, llm.ProviderIdentityTransport, false, 0, "", false, nil,
		)
		if evidenceErr != nil {
			return llm.ProviderIdentityOperationEvidence{}, evidenceErr
		}
		return operation, c.wrapConnectivityError(err, spec.endpoint)
	}
	defer response.Body.Close()
	encodingCount, contentEncoding, uncompressed := exactProviderContentEncodingEvidence(response)
	raw, readErr := io.ReadAll(io.LimitReader(
		response.Body, llm.MaxProviderIdentityComponentBytes+1,
	))
	if readErr != nil {
		operation, evidenceErr := spec.evidence(
			response.StatusCode, llm.ProviderIdentityBodyReadError, false,
			encodingCount, contentEncoding, uncompressed, raw,
		)
		if evidenceErr != nil {
			return llm.ProviderIdentityOperationEvidence{}, evidenceErr
		}
		return operation, fmt.Errorf(
			"read Ollama identity response %s: captured_bytes=%d captured_sha256=%s: %w",
			spec.endpoint, len(raw), operation.ResponseSHA256, readErr,
		)
	}
	if len(raw) > llm.MaxProviderIdentityComponentBytes {
		operation, evidenceErr := spec.evidence(
			response.StatusCode, llm.ProviderIdentityBodyLimit, false,
			encodingCount, contentEncoding, uncompressed, raw,
		)
		if evidenceErr != nil {
			return llm.ProviderIdentityOperationEvidence{}, evidenceErr
		}
		return operation, fmt.Errorf(
			"Ollama identity response %s exceeds %d bytes; captured_sha256=%s",
			spec.endpoint, llm.MaxProviderIdentityComponentBytes, operation.ResponseSHA256,
		)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		operation, evidenceErr := spec.evidence(
			response.StatusCode, llm.ProviderIdentityHTTPError, true,
			encodingCount, contentEncoding, uncompressed, raw,
		)
		if evidenceErr != nil {
			return llm.ProviderIdentityOperationEvidence{}, evidenceErr
		}
		return operation, fmt.Errorf(
			"Ollama identity request %s failed: status=%d body_bytes=%d body_sha256=%s",
			spec.endpoint, response.StatusCode, len(raw), operation.ResponseSHA256,
		)
	}
	if !exactProviderContentEncoding(response) {
		operation, evidenceErr := spec.evidence(
			response.StatusCode, llm.ProviderIdentityInvalidJSON, true,
			encodingCount, contentEncoding, uncompressed, raw,
		)
		if evidenceErr != nil {
			return llm.ProviderIdentityOperationEvidence{}, evidenceErr
		}
		return operation, fmt.Errorf(
			"Ollama identity response %s used unsupported content encoding %q",
			spec.endpoint, response.Header.Get("Content-Encoding"),
		)
	}
	if err := exactjson.ValidateUniqueObject(raw, "Ollama identity response "+spec.endpoint); err != nil {
		operation, evidenceErr := spec.evidence(
			response.StatusCode, llm.ProviderIdentityInvalidJSON, true,
			encodingCount, contentEncoding, uncompressed, raw,
		)
		if evidenceErr != nil {
			return llm.ProviderIdentityOperationEvidence{}, evidenceErr
		}
		return operation, fmt.Errorf("Ollama identity response %s is inexact: %w", spec.endpoint, err)
	}
	operation, err := spec.evidence(
		response.StatusCode, llm.ProviderIdentitySucceeded, true,
		encodingCount, contentEncoding, uncompressed, raw,
	)
	if err != nil {
		return llm.ProviderIdentityOperationEvidence{}, err
	}
	return operation, nil
}

func (spec providerIdentityOperationSpec) evidence(
	status int,
	disposition llm.ProviderIdentityOperationDisposition,
	complete bool,
	contentEncodingCount int,
	contentEncoding string,
	uncompressed bool,
	response []byte,
) (llm.ProviderIdentityOperationEvidence, error) {
	return llm.NewProviderIdentityOperationEvidence(
		spec.operation, spec.method, spec.endpoint, true, spec.request,
		status, disposition, complete, contentEncodingCount, contentEncoding,
		uncompressed, response,
	)
}
