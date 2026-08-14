package ollama

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/gryph/omnidex/internal/llm"
)

func (c *Client) generatePreparedRaw(
	ctx context.Context,
	prepared llm.PreparedModel,
	observed llm.ObservedProviderIdentity,
) (llm.PreparedGeneration, error) {
	payload, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	digest := sha256.Sum256(payload)
	result := llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		Protocol:                   prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
		ProviderRequestSHA256:      hex.EncodeToString(digest[:]),
		ProviderObservation:        observed.Observation,
		ProviderIdentityEvidence:   observed.Evidence,
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(payload),
	)
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, disposition, err := c.doExactProviderRequest(request)
	result.ProviderRequestDisposition = disposition
	if err != nil {
		result.ProviderResponseDisposition = llm.ProviderResponseTransportError
		return result, c.wrapConnectivityError(err, "/api/generate")
	}
	defer response.Body.Close()
	result.ProviderHTTPStatus = response.StatusCode
	result.ProviderContentEncoding = exactProviderContentEncodingEvidence(response)
	body, readErr := io.ReadAll(io.LimitReader(
		response.Body, llm.MaxExactPreparedProviderResponseBytes+1,
	))
	result.ProviderResponseCapturedBytes = len(body)
	result.ProviderResponseCapture = append([]byte{}, body...)
	captureDigest := sha256.Sum256(body)
	result.ProviderResponseCaptureSHA256 = hex.EncodeToString(captureDigest[:])
	if readErr != nil {
		result.ProviderResponseDisposition = llm.ProviderResponseBodyReadError
		return result, readErr
	}
	if len(body) > llm.MaxExactPreparedProviderResponseBytes {
		result.ProviderResponseDisposition = llm.ProviderResponseBodyLimit
		return result, fmt.Errorf(
			"ollama raw generation response exceeds %d bytes",
			llm.MaxExactPreparedProviderResponseBytes,
		)
	}
	result.ProviderResponseComplete = true
	result.ProviderResponseBytesKnown = true
	result.ProviderResponseBytes = int64(len(body))
	result.ProviderResponseSHA256 = result.ProviderResponseCaptureSHA256
	if !exactProviderContentEncoding(response) {
		result.ProviderResponseDisposition = llm.ProviderResponseInvalidJSON
		return result, fmt.Errorf(
			"exact Ollama response used unsupported content encoding: values=%d bytes=%d sha256=%s uncompressed=%t",
			result.ProviderContentEncoding.Values, result.ProviderContentEncoding.Bytes,
			result.ProviderContentEncoding.SHA256, result.ProviderContentEncoding.Uncompressed,
		)
	}
	decoded, decodeErr := llm.DecodeExactPreparedResponseForProtocol(
		prepared.Protocol, response.StatusCode, body,
	)
	result.ProviderResponseDisposition = decoded.Disposition
	result.ProviderResponseModel = decoded.Model
	result.Content = decoded.Content
	result.Thinking = decoded.Thinking
	result.ProviderDonePresent = decoded.DonePresent
	result.ProviderDone = decoded.Done
	result.ProviderDoneReason = decoded.DoneReason
	result.UsagePresent = decoded.UsagePresent
	result.Usage = decoded.Usage
	if decodeErr != nil {
		return result, fmt.Errorf("exact Ollama raw generation response: %w", decodeErr)
	}
	return result, nil
}
