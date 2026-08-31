package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func validateExactStationStaticCall(
	prompt string,
	contract llmResponseContract,
	contextTokens int,
) error {
	if err := contract.Protocol.Validate(); err != nil {
		return err
	}
	if err := contract.OutputLimitMode.Validate(); err != nil {
		return err
	}
	if contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
		return fmt.Errorf("portable exact station requires natural output completion")
	}
	if contract.Protocol != llm.ExactPreparedProtocolRawTextV2 {
		return fmt.Errorf("exact portable station requires raw text")
	}
	input, err := llm.ExactPreparedModelInput(prompt, contract.PromptHint)
	if err != nil {
		return err
	}
	return llm.ValidateExactPreparedNaturalInputAuthority(
		contextTokens,
		input,
	)
}

func prepareExactStationCall(
	call exactStationCall,
	contract llmResponseContract,
	modelName string,
	temperature *llm.ExactPreparedTemperature,
) (llm.PreparedModel, error) {
	if call.OutputLimitMode != contract.OutputLimitMode {
		return llm.PreparedModel{}, fmt.Errorf(
			"station call output-limit mode %q differs from response contract %q",
			call.OutputLimitMode, contract.OutputLimitMode,
		)
	}
	if temperature == nil {
		value := llm.ExactPreparedTemperature(0)
		temperature = &value
	}
	stop, err := directStationStopSequence(call.WorkKind)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	return llm.PreparedModel{
		Protocol: contract.Protocol, BaseModel: modelName, ContextModel: modelName,
		Prompt: call.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: call.MaxOutputTokens, OutputLimitMode: call.OutputLimitMode,
		ContextTokens: call.ContextTokens, RawTextStopSequence: stop,
		Temperature: temperature,
	}, nil
}

func directStationStopSequence(kind assemblyline.WorkKind) (string, error) {
	framing, err := assemblyline.PortableResponseFramingForWorkKind(kind)
	if err != nil {
		return "", fmt.Errorf("resolve station response framing: %w", err)
	}
	switch framing {
	case assemblyline.PortableResponseFramingSingleLine:
		return llm.ExactPreparedLineStopV1, nil
	case assemblyline.PortableResponseFramingNaturalMultiline:
		return "", nil
	default:
		return "", fmt.Errorf("station response framing %q is not registered", framing)
	}
}
