package worker

import (
	"fmt"
	"unicode/utf8"
)

type boundedCommandOutput struct {
	prefix   []byte
	observed int64
	limit    int
}

func newBoundedCommandOutput(limit int) (*boundedCommandOutput, error) {
	if limit < 1 {
		return nil, fmt.Errorf("command output bound must be positive")
	}
	return &boundedCommandOutput{prefix: make([]byte, 0, limit), limit: limit}, nil
}

func (output *boundedCommandOutput) Write(value []byte) (int, error) {
	if output == nil || output.limit < 1 {
		return 0, fmt.Errorf("command output writer is not initialized")
	}
	output.observed += int64(len(value))
	remaining := output.limit - len(output.prefix)
	if remaining > len(value) {
		remaining = len(value)
	}
	if remaining > 0 {
		output.prefix = append(output.prefix, value[:remaining]...)
	}
	return len(value), nil
}

func (output *boundedCommandOutput) Result() (text string, observed int64, truncated bool) {
	if output == nil {
		return "", 0, false
	}
	prefix := output.prefix
	if !utf8.Valid(prefix) {
		for len(prefix) > 0 && !utf8.Valid(prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return string(prefix), output.observed, output.observed > int64(len(prefix))
}
