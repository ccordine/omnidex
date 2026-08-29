package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

const ollamaProgressPersistenceInterval = 250 * time.Millisecond

func (s *Server) resumeOllamaModelDownloads() {
	go func() {
		items, err := s.ollamaDownloads.ListActiveOllamaModelDownloads(s.lifecycleContext)
		if err != nil {
			log.Printf("resume Ollama model downloads failed: %v", err)
			s.publishOllamaDownloadProblem("Download recovery failed: " + durableOllamaDownloadError(err))
			return
		}
		for _, item := range items {
			s.launchOllamaModelDownload(item)
		}
	}()
}

func (s *Server) launchOllamaModelDownload(item queue.OllamaModelDownload) bool {
	if err := item.Validate(); err != nil {
		log.Printf("reject invalid active Ollama model download id=%q: %v", item.ID, err)
		return false
	}
	if item.State != queue.OllamaModelDownloadQueued && item.State != queue.OllamaModelDownloadRunning {
		log.Printf("reject terminal Ollama model download scheduling id=%q state=%q", item.ID, item.State)
		return false
	}
	s.ollamaDownloadMu.Lock()
	if _, exists := s.ollamaDownloadRunning[item.ID]; exists {
		s.ollamaDownloadMu.Unlock()
		return false
	}
	s.ollamaDownloadRunning[item.ID] = struct{}{}
	s.ollamaDownloadMu.Unlock()
	go s.runOllamaModelDownload(item)
	return true
}

func (s *Server) runOllamaModelDownload(item queue.OllamaModelDownload) {
	defer func() {
		s.ollamaDownloadMu.Lock()
		delete(s.ollamaDownloadRunning, item.ID)
		s.ollamaDownloadMu.Unlock()
	}()
	select {
	case s.ollamaDownloadSlots <- struct{}{}:
		defer func() { <-s.ollamaDownloadSlots }()
	case <-s.lifecycleContext.Done():
		return
	}
	running, err := s.ollamaDownloads.StartOllamaModelDownload(s.lifecycleContext, item.ID)
	if err != nil {
		s.failOllamaModelDownload(item, fmt.Errorf("start durable download: %w", err))
		return
	}
	s.publishOllamaDownload(running, "", "")

	client := s.ollamaLifecycleClient()
	var previous ollama.PullProgress
	var lastPersisted time.Time
	err = client.PullModelProgress(s.lifecycleContext, item.Model, func(progress ollama.PullProgress) error {
		now := time.Now()
		if !shouldPersistOllamaProgress(previous, progress, lastPersisted, now) {
			previous = progress
			return nil
		}
		updated, persistErr := s.ollamaDownloads.RecordOllamaModelDownloadProgress(
			s.lifecycleContext, item.ID, progress,
		)
		if persistErr != nil {
			return persistErr
		}
		previous, lastPersisted = progress, now
		s.publishOllamaDownload(updated, "", "")
		return nil
	})
	if err == nil {
		installed, verifyErr := client.HasModel(s.lifecycleContext, item.Model)
		if verifyErr != nil {
			err = fmt.Errorf("verify installed model: %w", verifyErr)
		} else if !installed {
			err = fmt.Errorf("Ollama pull completed but model %q is not installed", item.Model)
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) && s.lifecycleContext.Err() != nil {
			return
		}
		s.failOllamaModelDownload(item, err)
		return
	}
	completed, err := s.ollamaDownloads.CompleteOllamaModelDownload(s.lifecycleContext, item.ID)
	if err != nil {
		s.failOllamaModelDownload(item, fmt.Errorf("commit installed model: %w", err))
		return
	}
	s.publishOllamaDownload(completed, "Installed "+completed.Model, "ok")
}

func shouldPersistOllamaProgress(
	previous, current ollama.PullProgress,
	lastPersisted, now time.Time,
) bool {
	return lastPersisted.IsZero() || current.Status != previous.Status ||
		current.Digest != previous.Digest || current.Status == "success" ||
		(current.Total > 0 && current.Completed == current.Total) ||
		now.Sub(lastPersisted) >= ollamaProgressPersistenceInterval
}

func (s *Server) failOllamaModelDownload(item queue.OllamaModelDownload, cause error) {
	reason := durableOllamaDownloadError(cause)
	failed, err := s.ollamaDownloads.FailOllamaModelDownload(
		s.lifecycleContext, item.ID, reason,
	)
	if err != nil {
		log.Printf("persist failed Ollama download id=%s model=%q cause=%v persistence=%v", item.ID, item.Model, cause, err)
		s.publishOllamaDownloadProblem("Could not persist failed download " + item.Model)
		return
	}
	log.Printf("Ollama model download failed id=%s model=%q: %v", item.ID, item.Model, cause)
	s.publishOllamaDownload(failed, "Download failed for "+failed.Model, "error")
}

func durableOllamaDownloadError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) <= 2048 {
		return message
	}
	digest := sha256.Sum256([]byte(message))
	return "Ollama download error exceeded 2048 bytes; sha256=" + hex.EncodeToString(digest[:])
}

func (s *Server) publishOllamaDownload(item queue.OllamaModelDownload, toast, tone string) {
	s.broadcastRealtime([]string{realtimeTopicUI}, realtimeMessage{
		EventName: "ollama-download", StateKey: "ollama-download:" + item.ID,
		Reason: string(item.State), Summary: item.Model + ": " + item.Status,
		Toast: toast, ToastTone: tone,
	})
}

func (s *Server) publishOllamaDownloadProblem(message string) {
	s.broadcastRealtime([]string{realtimeTopicUI}, realtimeMessage{
		EventName: "ollama-download", Reason: "coordinator_error", Summary: message,
		Toast: message, ToastTone: "error",
	})
}
