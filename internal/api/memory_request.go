package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
)

const (
	maxMemoryRequestBodyBytes int64 = 2 << 20
	maxMemoryBatchBodyBytes   int64 = 16 << 20
	maxMemoryBatchItems             = 512
)

type memoryRequest struct {
	Scope      model.MemoryScope      `json:"scope"`
	Source     model.MemorySource     `json:"source"`
	Kind       model.MemoryKind       `json:"kind"`
	Content    string                 `json:"content"`
	Tags       []string               `json:"tags"`
	Categories []model.MemoryCategory `json:"categories"`
}

func parseMemoryScope(projectValue, channelValue string) (model.MemoryScope, error) {
	if projectValue == "" || projectValue != strings.TrimSpace(projectValue) {
		return model.MemoryScope{}, fmt.Errorf("memory project_id must be exact positive decimal text")
	}
	projectID, err := strconv.ParseInt(projectValue, 10, 64)
	if err != nil {
		return model.MemoryScope{}, fmt.Errorf("memory project_id must be exact positive decimal text")
	}
	scope := model.MemoryScope{ProjectID: projectID, ChannelID: model.ChannelID(channelValue)}
	if err := scope.Validate(); err != nil {
		return model.MemoryScope{}, err
	}
	return scope, nil
}

type memoryBatchRequest struct {
	Memories []memoryRequest `json:"memories"`
}

func (request memoryRequest) input() model.MemoryInput {
	return model.MemoryInput{
		Scope: request.Scope, Source: request.Source, Kind: request.Kind, Content: request.Content,
		Tags:       append([]string(nil), request.Tags...),
		Categories: append([]model.MemoryCategory(nil), request.Categories...),
	}
}

func decodeExactMemoryJSON(
	w http.ResponseWriter,
	r *http.Request,
	name string,
	maxBytes int64,
	destination any,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%s exceeds the %d-byte transport bound", name, maxBytes)
		}
		return fmt.Errorf("read %s JSON: %w", name, err)
	}
	if err := exactjson.ValidateObject(raw, destination, name); err != nil {
		return fmt.Errorf("invalid %s JSON: %w", name, err)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return fmt.Errorf("decode %s JSON: %w", name, err)
	}
	return nil
}

func validateMemoryBatchRequest(request memoryBatchRequest) ([]model.MemoryInput, error) {
	if len(request.Memories) == 0 || len(request.Memories) > maxMemoryBatchItems {
		return nil, fmt.Errorf("memory batch must contain 1..%d items", maxMemoryBatchItems)
	}
	inputs := make([]model.MemoryInput, len(request.Memories))
	for index, memory := range request.Memories {
		input := memory.input()
		if err := input.Validate(); err != nil {
			return nil, fmt.Errorf("memory batch item %d: %w", index+1, err)
		}
		inputs[index] = input
	}
	return inputs, nil
}
