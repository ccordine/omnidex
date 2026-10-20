// Package codex translates one Omnidex semantic request to the signed-in
// local Codex transport. Omnidex retains tools, state, and workflow authority.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type Client struct {
	command   string
	directory string
	timeout   time.Duration
	parser    *parserProcess
}

func New(command, directory string, timeout time.Duration) (*Client, error) {
	if command == "" || directory == "" || timeout <= 0 {
		return nil, fmt.Errorf("Codex transport requires an executable, working directory, and timeout")
	}
	return &Client{command: resolveCodexExecutable(command), directory: directory, timeout: timeout}, nil
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
	return client.GeneratePreparedExactWithActivity(parent, prepared, nil)
}

// GeneratePreparedExactWithActivity observes public native items for this
// semantic call only. Observation cannot change its request or result contract.
func (client *Client) GeneratePreparedExactWithActivity(parent context.Context, prepared llm.PreparedModel, observer func(model.AgentActivity) error) (llm.PreparedGeneration, error) {
	result := llm.PreparedGeneration{Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched, ProviderResponseDisposition: llm.ProviderResponseTransportError}
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
	connection, release, err := client.generationConnection(ctx)
	if err != nil {
		return result, err
	}
	healthy := false
	defer func() { release(healthy) }()
	// Cancel blocked transport reads as well as active inference.
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = connection.process.Cancel(); close(interrupted) })
	defer func() {
		if !stop() {
			<-interrupted
		}
	}()
	if client.parser != nil && (request.Continuation != nil || request.RetainContext) {
		return result, fmt.Errorf("user parser requires fresh ephemeral threads")
	}
	capture, dispatched, err := connection.generate(ctx, request, observer)
	healthy = err == nil
	if client.parser != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
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
