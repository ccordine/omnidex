package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func prepareExactStationCall(
	call exactStationCall,
	modelName string,
	temperature *llm.ExactPreparedTemperature,
) (llm.PreparedModel, error) {
	if temperature == nil {
		value := llm.ExactPreparedTemperature(0)
		temperature = &value
	}
	stop, err := directStationStopSequence(call.WorkKind)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	return llm.PreparedModel{
		Protocol: llm.ExactPreparedProtocolRawTextV2, BaseModel: modelName, ContextModel: modelName,
		Prompt: call.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: call.MaxOutputTokens, OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
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
