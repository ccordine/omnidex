package llm

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

// CodexContinuation identifies native retained context, not invented token IDs.
// The exact parent response and existing queue lineage authorize its reuse.
type CodexContinuation struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Model    string `json:"model"`
}

type CodexStationRequest struct {
	Model           string             `json:"model"`
	Prompt          string             `json:"prompt"`
	ContextTokens   int                `json:"contextTokens"`
	MaxOutputTokens int                `json:"maxOutputTokens"`
	Continuation    *CodexContinuation `json:"continuation,omitempty"`
	RetainContext   bool               `json:"retainContext"`
}

func validateCodexPreparedRequest(prepared PreparedModel) error {
	if prepared.Temperature != nil || prepared.RawTextStopSequence != "" || prepared.RetainedContext != nil {
		return fmt.Errorf("Codex request contains unsupported native provider controls")
	}
	if prepared.BaseModel == "" || prepared.ContextModel != prepared.BaseModel || prepared.OutputLimitMode != ExactPreparedOutputLimitExplicit {
		return fmt.Errorf("Codex request has an invalid model or output authority")
	}
	if err := ValidateExactPreparedInputAuthority(prepared.ContextTokens, prepared.MaxOutputTokens, prepared.Prompt); err != nil {
		return err
	}
	if len(prepared.RetainedProviderContext) != 0 {
		if !prepared.RetainProviderContext {
			return fmt.Errorf("Codex continuation requires retained-context authority")
		}
		continuation, err := ParseCodexContinuation(prepared.RetainedProviderContext)
		if err != nil || continuation.Model != prepared.BaseModel {
			return fmt.Errorf("Codex continuation differs from its immutable model")
		}
	}
	return nil
}

func ParseCodexContinuation(encoded []byte) (CodexContinuation, error) {
	var result CodexContinuation
	if len(encoded) > 2048 {
		return result, fmt.Errorf("Codex continuation exceeds its bound")
	}
	if err := exactjson.ValidateCompatibleObject(encoded, result, "Codex continuation"); err != nil {
		return result, err
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return result, err
	}
	if result.ThreadID == "" || len(result.ThreadID) > 256 || result.TurnID == "" || len(result.TurnID) > 256 || result.Model == "" || len(result.Model) > 512 {
		return result, fmt.Errorf("Codex continuation is incomplete")
	}
	return result, nil
}

func codexPreparedRequestBytes(prepared PreparedModel) ([]byte, error) {
	request := CodexStationRequest{Model: prepared.BaseModel, Prompt: prepared.Prompt,
		ContextTokens: prepared.ContextTokens, MaxOutputTokens: prepared.MaxOutputTokens, RetainContext: prepared.RetainProviderContext}
	if len(prepared.RetainedProviderContext) != 0 {
		continuation, err := ParseCodexContinuation(prepared.RetainedProviderContext)
		if err != nil {
			return nil, err
		}
		request.Continuation = &continuation
	}
	return exactjson.Canonical(request)
}

func validPreparedResponseStatus(protocol ExactPreparedProtocol, status int) bool {
	if protocol == ExactPreparedProtocolCodexV1 {
		return status == 0
	}
	return status >= 100 && status <= 599
}

func validPreparedDoneReason(protocol ExactPreparedProtocol, reason string) bool {
	if protocol == ExactPreparedProtocolCodexV1 {
		return reason == "completed"
	}
	return reason == "stop" || reason == "length"
}

func validPreparedSuccessStatus(protocol ExactPreparedProtocol, status int) bool {
	if protocol == ExactPreparedProtocolCodexV1 {
		return status == 0
	}
	return status >= 200 && status < 300
}
