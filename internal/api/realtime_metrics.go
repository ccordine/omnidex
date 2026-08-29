package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

const telemetryNotifyChannel = "omni_telemetry"
const telemetryRealtimeWindow = 250 * time.Millisecond

var ErrRealtimeLifecycleUnavailable = errors.New("realtime lifecycle context is not initialized")

type telemetryNotifyPayload struct {
	EventType string          `json:"event_type"`
	RunID     string          `json:"run_id"`
	JobID     int64           `json:"job_id"`
	Message   string          `json:"message"`
	Payload   json.RawMessage `json:"payload"`
}

func parseTelemetryNotifyPayload(raw string) (telemetryNotifyPayload, error) {
	var payload telemetryNotifyPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return telemetryNotifyPayload{}, fmt.Errorf("decode telemetry notification: %w", err)
	}
	payload.EventType = strings.TrimSpace(payload.EventType)
	payload.RunID = strings.TrimSpace(payload.RunID)
	payload.Message = strings.TrimSpace(payload.Message)
	if payload.EventType == "" {
		return telemetryNotifyPayload{}, errors.New("telemetry notification requires event_type")
	}
	nested := bytes.TrimSpace(payload.Payload)
	if len(nested) > 0 && nested[0] == '{' {
		var details struct {
			Message string `json:"message"`
			JobID   int64  `json:"job_id"`
		}
		if err := json.Unmarshal(nested, &details); err != nil {
			return telemetryNotifyPayload{}, fmt.Errorf("decode telemetry notification payload: %w", err)
		}
		if payload.Message == "" {
			payload.Message = strings.TrimSpace(details.Message)
		}
		if payload.JobID <= 0 {
			payload.JobID = details.JobID
		}
	}
	return payload, nil
}

func telemetryJobProgress(payload telemetryNotifyPayload) (realtimeJobPhase, string, bool) {
	if payload.JobID <= 0 {
		return "", "", false
	}
	switch payload.EventType {
	case "run_completed":
		return realtimeJobFinished, "Job completed", true
	case "run_failed":
		return realtimeJobFinished, "Job failed", true
	case "run_cancelled":
		return realtimeJobFinished, "Job canceled", true
	}
	if message := strings.TrimSpace(payload.Message); message != "" {
		return realtimeJobChanged, queue.TruncateUTF8Text(message, 180, "…"), true
	}
	summary := strings.TrimSpace(strings.ReplaceAll(payload.EventType, "_", " "))
	if summary == "" {
		return "", "", false
	}
	return realtimeJobChanged, summary, true
}

func (s *Server) startRealtimeTelemetryListener(ctx context.Context) {
	if s.repo == nil {
		return
	}
	coalescer, err := s.ensureTelemetryRealtimeCoalescer()
	if err != nil {
		log.Printf("realtime telemetry listener rejected: %v", err)
		return
	}
	go func() {
		<-ctx.Done()
		coalescer.Stop()
	}()
	go s.listenTelemetryNotifications(ctx)
}

func (s *Server) ensureTelemetryRealtimeCoalescer() (*telemetryRealtimeCoalescer, error) {
	if s.lifecycleContext == nil {
		return nil, ErrRealtimeLifecycleUnavailable
	}
	s.telemetryRealtimeOnce.Do(func() {
		s.telemetryRealtime = newTelemetryRealtimeCoalescer(telemetryRealtimeWindow, func(trigger telemetryNotifyPayload) {
			ctx, cancel := context.WithTimeout(s.lifecycleContext, 10*time.Second)
			defer cancel()
			s.publishMetricsGlance(ctx, trigger)
		})
	})
	if s.telemetryRealtime == nil {
		return nil, errors.New("realtime telemetry coalescer initialization failed")
	}
	return s.telemetryRealtime, nil
}

func (s *Server) listenTelemetryNotifications(ctx context.Context) {
	backoff := 250 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.runTelemetryListener(ctx); err != nil && ctx.Err() == nil {
			if errors.Is(err, queue.ErrRepositoryNotConfigured) {
				log.Printf("realtime telemetry listener stopped: %v", err)
				return
			}
			log.Printf("realtime telemetry listener disconnected; retrying in %s: %v", backoff, err)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
	}
}

func (s *Server) runTelemetryListener(ctx context.Context) error {
	conn, err := s.repo.AcquireNotifyConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	if err := conn.Exec(ctx, "LISTEN "+telemetryNotifyChannel); err != nil {
		return err
	}

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		payload, err := parseTelemetryNotifyPayload(notification.Payload)
		if err != nil {
			log.Printf("realtime telemetry notification rejected payload_bytes=%d: %v", len(notification.Payload), err)
			continue
		}
		if phase, summary, ok := telemetryJobProgress(payload); ok {
			s.publishJobProgress(payload.JobID, phase, summary)
		}
		coalescer, err := s.ensureTelemetryRealtimeCoalescer()
		if err != nil {
			return err
		}
		if err := coalescer.Signal(payload); err != nil {
			return fmt.Errorf("schedule realtime metrics refresh: %w", err)
		}
	}
}

func (s *Server) publishMetricsGlance(ctx context.Context, trigger telemetryNotifyPayload) {
	if s.repo == nil {
		return
	}
	glance, err := s.repo.TelemetryGlance(ctx)
	if err != nil {
		log.Printf("realtime metrics glance refresh failed trigger=%q run=%q: %v", trigger.EventType, trigger.RunID, err)
		return
	}
	msg := s.buildMetricsGlanceRealtimeMessage(glance, trigger)
	s.broadcastRealtime([]string{realtimeTopicUI, realtimeTopicMetrics}, msg)
}

func (s *Server) buildMetricsGlanceRealtimeMessage(glance queue.TelemetryGlanceSummary, trigger telemetryNotifyPayload) realtimeMessage {
	markup := renderMetricsNavBadgesHTML(glance)
	msg := realtimeMessage{
		HTML:      markup,
		EventName: "metrics-glance",
		StateKey:  "metrics-glance",
	}
	if trigger.EventType != "" && queue.IsTelemetryStruggleEvent(trigger.EventType) {
		label := strings.ReplaceAll(trigger.EventType, "_", " ")
		if trigger.Message != "" {
			msg.Toast = fmt.Sprintf("%s — %s", label, trigger.Message)
		} else {
			msg.Toast = label
		}
		msg.ToastTone = "error"
	}
	return msg
}

func renderMetricsNavBadgesHTML(glance queue.TelemetryGlanceSummary) string {
	parts := []string{}
	if glance.LiveRuns > 0 {
		parts = append(parts, fmt.Sprintf(
			`<span class="inline-flex min-w-[1.25rem] items-center justify-center rounded-full border border-cyan-300/30 bg-cyan-300/10 px-1.5 py-0.5 text-[10px] font-semibold text-cyan-100" title="Live runs">%s</span>`,
			html.EscapeString(fmt.Sprintf("%d", glance.LiveRuns)),
		))
	}
	if glance.RecentErrors > 0 {
		parts = append(parts, fmt.Sprintf(
			`<span class="inline-flex min-w-[1.25rem] items-center justify-center rounded-full border border-rose-400/35 bg-rose-950/80 px-1.5 py-0.5 text-[10px] font-semibold text-rose-100" title="Errors in the last hour">%s</span>`,
			html.EscapeString(fmt.Sprintf("%d", glance.RecentErrors)),
		))
	} else if glance.Struggling && glance.StruggleSignals > 0 {
		parts = append(parts, fmt.Sprintf(
			`<span class="inline-flex min-w-[1.25rem] items-center justify-center rounded-full border border-amber-300/30 bg-amber-300/10 px-1.5 py-0.5 text-[10px] font-semibold text-amber-100" title="Struggle signals (7d)">%s</span>`,
			html.EscapeString(fmt.Sprintf("%d", glance.StruggleSignals)),
		))
	}
	inner := strings.Join(parts, "")
	if inner == "" {
		inner = `<span class="text-zinc-500">05</span>`
	}
	return `<template data-recyclr-target="metrics-nav-badges"><span class="flex items-center gap-1.5">` + inner + `</span></template>`
}

func (s *Server) handleMetricsGlance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository unavailable")
		return
	}
	glance, err := s.repo.TelemetryGlance(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, glance)
}
