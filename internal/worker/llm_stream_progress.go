package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

const (
	llmProgressInitialBytes = 256
	llmProgressByteStep     = 1024
	llmProgressMaxSilence   = 15 * time.Second
)

func (s *Service) generatePreparedWithProgress(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	scope string,
	prepared llm.PreparedModel,
) (string, error) {
	streaming, ok := s.llm.(llm.PreparedStreamingClient)
	if !ok {
		s.emitStepEvent(authority, "llm_progress_unavailable", "scope="+safeLine(scope, "generation")+" provider=non_streaming")
		return s.llm.GeneratePrepared(ctx, prepared)
	}
	started := time.Now()
	lastSeen := 0
	lastEmitted := 0
	lastEmission := started
	return streaming.GeneratePreparedStream(ctx, prepared, func(progress llm.GenerationProgress) error {
		if progress.OutputBytes <= lastSeen {
			return fmt.Errorf("streaming output byte count must increase: previous=%d current=%d", lastSeen, progress.OutputBytes)
		}
		lastSeen = progress.OutputBytes
		now := time.Now()
		if progress.OutputBytes < llmProgressInitialBytes && now.Sub(lastEmission) < llmProgressMaxSilence {
			return nil
		}
		if progress.OutputBytes-lastEmitted < llmProgressByteStep && now.Sub(lastEmission) < llmProgressMaxSilence {
			return nil
		}
		s.emitStepEvent(authority, "llm_stream_progress", fmt.Sprintf(
			"scope=%s output_bytes=%d elapsed=%s",
			safeLine(scope, "generation"), progress.OutputBytes, now.Sub(started).Truncate(time.Second),
		))
		lastEmitted = progress.OutputBytes
		lastEmission = now
		return nil
	})
}
