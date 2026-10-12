// Package codex translates one Omnidex semantic request to the signed-in
// local Codex transport. Omnidex retains tools, state, and workflow authority.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type Client struct {
	command   string
	directory string
	timeout   time.Duration
}

func New(command, directory string, timeout time.Duration) (*Client, error) {
	if command == "" || directory == "" || timeout <= 0 {
		return nil, fmt.Errorf("Codex transport requires an executable, working directory, and timeout")
	}
	return &Client{command: command, directory: directory, timeout: timeout}, nil
}

func (client *Client) PrepareExactRequest(prepared llm.PreparedModel) (llm.PreparedModel, error) {
	if prepared.RetainedContext != nil {
		return prepared, fmt.Errorf("Codex cannot consume another provider's native token context")
	}
	prepared.Protocol = llm.ExactPreparedProtocolCodexV1
	// These are Ollama sampling/decoder settings. The Codex protocol uses its
	// native reasoning configuration and the station's unchanged result parser.
	prepared.Temperature = nil
	prepared.RawTextStopSequence = ""
	_, err := llm.ExactPreparedRequestBytes(prepared)
	return prepared, err
}

func (client *Client) GeneratePreparedExact(parent context.Context, prepared llm.PreparedModel) (llm.PreparedGeneration, error) {
	result := llm.PreparedGeneration{Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched}
	if prepared.Protocol != llm.ExactPreparedProtocolCodexV1 {
		return result, fmt.Errorf("Codex requires its registered native request protocol")
	}
	encoded, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return result, err
	}
	var request llm.CodexStationRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	connection, err := startConnection(ctx, client.command, client.directory)
	if err != nil {
		return result, err
	}
	defer connection.close()
	capture, dispatched, err := connection.generate(ctx, request)
	if dispatched {
		result.ProviderRequestDisposition = llm.ProviderRequestDispatched
	}
	if err != nil {
		result.ProviderResponseDisposition = llm.ProviderResponseTransportError
		return result, err
	}
	body, err := json.Marshal(capture)
	if err != nil {
		return result, err
	}
	result.ProviderResponseCapture = body
	result.ProviderResponseCapturedBytes = len(body)
	result.ProviderResponseBytes = int64(len(body))
	result.ProviderResponseBytesKnown = true
	result.ProviderResponseComplete = true
	result.ProviderContentEncoding = llm.ProviderEncodingIdentity
	decoded, decodeErr := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 0, body)
	result.ProviderResponseDisposition, result.Content = decoded.Disposition, decoded.Content
	result.ProviderDonePresent, result.ProviderDone, result.ProviderDoneReason = decoded.DonePresent, decoded.Done, decoded.DoneReason
	result.UsagePresent, result.Usage = decoded.UsagePresent, decoded.Usage
	return result, decodeErr
}
