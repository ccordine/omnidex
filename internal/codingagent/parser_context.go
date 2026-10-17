package codingagent

import "encoding/json"

func RenderParserContext(call Assignment) (string, error) {
	if call.ParserStep != nil {
		return renderParserStepContext(call)
	}
	encoded := call.Context
	if encoded == "" {
		raw, err := json.Marshal(PromptContext(call))
		if err != nil {
			return "", err
		}
		encoded = string(raw)
	}
	context := "Shared context (background, not a new request):"
	if call.Role == ReviewResponse {
		references, err := json.Marshal(ReviewReferences(call))
		if err != nil {
			return "", err
		}
		contract, err := json.Marshal(ParserOutputSchema(call))
		if err != nil {
			return "", err
		}
		context += "\n\nReview result JSON Schema:\n" + string(contract) + "\n\nReview reference table:\n" + string(references)
	}
	if !call.Role.IsNative() && call.Role != ReviewResponse {
		contract, err := json.Marshal(ParserOutputSchema(call))
		if err != nil {
			return "", err
		}
		context += "\n\nParser result JSON Schema:\n" + string(contract)
	}
	return context + "\n\nAssignment:\n" + encoded, nil
}
