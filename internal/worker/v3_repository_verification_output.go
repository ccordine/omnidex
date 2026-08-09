package worker

import (
	"bytes"
	"fmt"
)

const (
	maxRepositoryGoVerificationStdoutBytes = 4 << 20
	maxRepositoryGoVerificationStderrBytes = 1 << 20
)

type exactRepositoryCommandOutput struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func newExactRepositoryCommandOutput(limit int) *exactRepositoryCommandOutput {
	return &exactRepositoryCommandOutput{limit: limit}
}

func (output *exactRepositoryCommandOutput) Write(content []byte) (int, error) {
	if output == nil || output.limit <= 0 {
		return 0, fmt.Errorf("repository verification output has no positive hard bound")
	}
	accepted := output.limit - output.buffer.Len()
	if accepted < 0 {
		accepted = 0
	}
	if accepted > len(content) {
		accepted = len(content)
	}
	if accepted > 0 {
		_, _ = output.buffer.Write(content[:accepted])
	}
	if accepted != len(content) {
		output.overflow = true
	}
	return len(content), nil
}

func (output *exactRepositoryCommandOutput) String() string {
	if output == nil {
		return ""
	}
	return output.buffer.String()
}

func (output *exactRepositoryCommandOutput) Validate(label string) error {
	if output == nil || output.limit <= 0 {
		return fmt.Errorf("%s has no exact output authority", label)
	}
	if output.overflow {
		return fmt.Errorf(
			"%s exceeded its exact %d-byte evidence bound",
			label, output.limit,
		)
	}
	return nil
}
