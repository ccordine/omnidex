package queue

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/modelref"
)

func (item OllamaModelDownload) Validate() error {
	if err := validateOllamaDownloadID(item.ID); err != nil {
		return err
	}
	if err := modelref.ValidateOllamaName(item.Model); err != nil {
		return err
	}
	if item.Status == "" || item.Status != strings.TrimSpace(item.Status) ||
		len(item.Status) > 512 || !utf8.ValidString(item.Status) ||
		item.Digest != strings.TrimSpace(item.Digest) || len(item.Digest) > 256 ||
		item.Error != strings.TrimSpace(item.Error) || len(item.Error) > 2048 ||
		!utf8.ValidString(item.Digest) || !utf8.ValidString(item.Error) ||
		item.TotalBytes < 0 || item.CompletedBytes < 0 ||
		(item.TotalBytes > 0 && item.CompletedBytes > item.TotalBytes) ||
		item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		return fmt.Errorf("persisted Ollama model download contains invalid fields")
	}
	switch item.State {
	case OllamaModelDownloadQueued:
		if item.StartedAt != nil || item.FinishedAt != nil || item.Error != "" {
			return fmt.Errorf("queued Ollama model download has invalid lifecycle")
		}
	case OllamaModelDownloadRunning:
		if item.StartedAt == nil || item.FinishedAt != nil || item.Error != "" {
			return fmt.Errorf("running Ollama model download has invalid lifecycle")
		}
	case OllamaModelDownloadCompleted:
		if item.StartedAt == nil || item.FinishedAt == nil || item.Error != "" {
			return fmt.Errorf("completed Ollama model download has invalid lifecycle")
		}
	case OllamaModelDownloadFailed:
		if item.FinishedAt == nil || item.Error == "" {
			return fmt.Errorf("failed Ollama model download has invalid lifecycle")
		}
	default:
		return fmt.Errorf("persisted Ollama model download state %q is invalid", item.State)
	}
	return nil
}
