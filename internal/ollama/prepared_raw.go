package ollama

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gryph/omnidex/internal/llm"
)

func (c *Client) generatePreparedRaw(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	payload, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	result := llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		Protocol:                   prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
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
	result.ProviderContentEncoding = exactProviderContentEncoding(response)
	body, readErr := io.ReadAll(io.LimitReader(
		response.Body, llm.MaxExactPreparedProviderResponseBytes+1,
	))
	result.ProviderResponseCapturedBytes = len(body)
	result.ProviderResponseCapture = append([]byte{}, body...)
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
	if !result.ProviderContentEncoding.IsIdentity() {
		result.ProviderResponseDisposition = llm.ProviderResponseInvalidJSON
		return result, fmt.Errorf(
			"exact Ollama response used unsupported content encoding",
		)
	}
	decoded, decodeErr := llm.DecodeExactPreparedResponseForProtocol(
		prepared.Protocol, response.StatusCode, body,
	)
	result.ProviderResponseDisposition = decoded.Disposition
	result.Content = decoded.Content
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
