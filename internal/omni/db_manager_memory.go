package omni

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

type MemoryRecord struct {
	ID            int64
	AgentID       string
	Source        string
	Kind          model.MemoryKind
	Content       string
	Tags          []string
	Priority      int
	SupersededAt  time.Time
	SupersededBy  int64
	StalenessNote string
	CreatedAt     time.Time
}

func memoryRecordFromInput(id int64, input model.MemoryInput) (MemoryRecord, error) {
	if id <= 0 {
		return MemoryRecord{}, fmt.Errorf("memory record identity must be positive")
	}
	if err := input.Validate(); err != nil {
		return MemoryRecord{}, err
	}
	return MemoryRecord{
		ID: id, AgentID: string(input.Source), Source: string(input.Source),
		Kind: input.Kind, Content: input.Content, Tags: append([]string(nil), input.Tags...),
	}, nil
}
