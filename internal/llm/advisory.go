package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type AdvisoryResponse struct {
	Thinking string `json:"thinking"`
	Content  string `json:"content"`
}

func (response AdvisoryResponse) Validate() error {
	if strings.TrimSpace(response.Thinking) == "" && strings.TrimSpace(response.Content) == "" {
		return fmt.Errorf("advisory response requires thinking or content")
	}
	return nil
}

func (response AdvisoryResponse) EvidenceJSON() (string, error) {
	if err := response.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("encode advisory response evidence: %w", err)
	}
	return string(raw), nil
}

func DecodeAdvisoryEvidence(raw string) (AdvisoryResponse, error) {
	var response AdvisoryResponse
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return AdvisoryResponse{}, fmt.Errorf("decode advisory response evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return AdvisoryResponse{}, fmt.Errorf("decode advisory response evidence: trailing JSON value")
		}
		return AdvisoryResponse{}, fmt.Errorf("decode advisory response evidence: %w", err)
	}
	if err := response.Validate(); err != nil {
		return AdvisoryResponse{}, err
	}
	return response, nil
}

type PreparedAdvisoryClient interface {
	GeneratePreparedAdvisory(context.Context, PreparedModel) (AdvisoryResponse, error)
}
