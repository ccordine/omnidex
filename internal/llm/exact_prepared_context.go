package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

// DecodeExactPreparedRetainedContext extracts native token IDs from the exact
// successful parent response at the first source-correction consumer. A
// response that needs no correction does not require continuation capability.
// The caller owns the persisted parent identity, successful receipt, model
// route and unchanged context limit; this function never reconstructs history.
func DecodeExactPreparedRetainedContext(
	protocol ExactPreparedProtocol,
	body []byte,
	contextTokens int,
) ([]int, error) {
	if err := protocol.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return nil, err
	}
	if len(body) > MaxExactPreparedProviderResponseBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("retained model context requires bounded UTF-8 provider evidence")
	}
	var wire struct {
		Context json.RawMessage `json:"context"`
	}
	if err := exactjson.ValidateCompatibleObject(body, wire, "retained model context response"); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(wire.Context))
	decoder.UseNumber()
	if start, err := decoder.Token(); err != nil || start != json.Delim('[') {
		return nil, fmt.Errorf("source correction requires a retained model context token array from its parent response")
	}
	var retained []int
	for decoder.More() {
		if len(retained) >= contextTokens {
			return nil, fmt.Errorf("retained model context exceeds %d native tokens", contextTokens)
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode retained model context token: %w", err)
		}
		number, ok := token.(json.Number)
		if !ok {
			return nil, fmt.Errorf("retained model context token %d is not an integer", len(retained))
		}
		id, err := strconv.Atoi(string(number))
		if err != nil || id < 0 {
			return nil, fmt.Errorf("retained model context token %d is not a nonnegative native integer", len(retained))
		}
		retained = append(retained, id)
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
		return nil, fmt.Errorf("retained model context token array is incomplete")
	}
	if err := validateExactPreparedRetainedContext(retained, contextTokens); err != nil {
		return nil, err
	}
	return retained, nil
}

func validateExactPreparedRetainedContext(retained []int, contextTokens int) error {
	if len(retained) == 0 || len(retained) > contextTokens {
		return fmt.Errorf("retained model context requires between one and %d native tokens", contextTokens)
	}
	for index, id := range retained {
		if id < 0 {
			return fmt.Errorf("retained model context token %d is negative", index)
		}
	}
	return nil
}
