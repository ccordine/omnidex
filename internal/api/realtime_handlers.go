package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/queue"
)

const telemetryNotifyChannel = "omni_telemetry"

var realtimeUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type telemetryNotifyPayload struct {
	EventType string         `json:"event_type"`
	RunID     string         `json:"run_id"`
	Message   string         `json:"message"`
	Payload   map[string]any `json:"payload"`
}

func parseTelemetryNotifyPayload(raw string) telemetryNotifyPayload {
	var payload telemetryNotifyPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		payload.EventType = strings.TrimSpace(raw)
		return payload
	}
	if payload.Message == "" && payload.Payload != nil {
		if msg, ok := payload.Payload["message"].(string); ok {
			payload.Message = strings.TrimSpace(msg)
		}
	}
	return payload
}

type realtimeMessage struct {
	ID        uint64 `json:"id"`
	HTML      string `json:"html"`
	EventName string `json:"eventName,omitempty"`
	Toast     string `json:"toast,omitempty"`
	ToastTone string `json:"toastTone,omitempty"`
	ProjectID int64  `json:"projectID,omitempty"`
	CardID    string `json:"cardID,omitempty"`
}

func (s *Server) ensureRealtimeHub() *RealtimeHub {
	if s.realtimeHub == nil {
		s.realtimeHub = NewRealtimeHub()
	}
	return s.realtimeHub
}

func (s *Server) startRealtimeTelemetryListener(ctx context.Context) {
	if s.repo == nil {
		return
	}
	go s.listenTelemetryNotifications(ctx)
}

func (s *Server) listenTelemetryNotifications(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.runTelemetryListener(ctx); err != nil && ctx.Err() == nil {
			time.Sleep(2 * time.Second)
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
		var payload telemetryNotifyPayload
		payload = parseTelemetryNotifyPayload(notification.Payload)
		s.publishMetricsGlance(ctx, payload)
	}
}

func (s *Server) nextRealtimeID() uint64 {
	return s.realtimeSeq.Add(1)
}

func (s *Server) publishMetricsGlance(ctx context.Context, trigger telemetryNotifyPayload) {
	if s.repo == nil {
		return
	}
	glance, err := s.repo.TelemetryGlance(ctx)
	if err != nil {
		return
	}
	msg := s.buildMetricsGlanceRealtimeMessage(glance, trigger)
	s.ensureRealtimeHub().Broadcast([]string{"ui", "metrics"}, msg)
}

func (s *Server) buildMetricsGlanceRealtimeMessage(glance queue.TelemetryGlanceSummary, trigger telemetryNotifyPayload) realtimeMessage {
	html := renderMetricsNavBadgesHTML(glance)
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		HTML:      html,
		EventName: "metrics-glance",
	}
	if trigger.EventType != "" && queue.IsTelemetryStruggleEvent(trigger.EventType) {
		label := strings.ReplaceAll(trigger.EventType, "_", " ")
		if trigger.Message != "" {
			msg.Toast = fmt.Sprintf("%s — %s", label, strings.TrimSpace(trigger.Message))
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

func (s *Server) handleRealtimeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository unavailable")
		return
	}

	conn, err := realtimeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	topics := parseRealtimeTopics(r.URL.Query().Get("topics"))
	_, outbound, unsubscribe := s.ensureRealtimeHub().Subscribe(topics)
	defer unsubscribe()

	if glance, err := s.repo.TelemetryGlance(r.Context()); err == nil {
		msg := s.buildMetricsGlanceRealtimeMessage(glance, telemetryNotifyPayload{})
		if data, err := json.Marshal(msg); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case payload, ok := <-outbound:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleRealtimeSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository unavailable")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	topics := parseRealtimeTopics(r.URL.Query().Get("topics"))
	_, outbound, unsubscribe := s.ensureRealtimeHub().Subscribe(topics)
	defer unsubscribe()

	if glance, err := s.repo.TelemetryGlance(r.Context()); err == nil {
		msg := s.buildMetricsGlanceRealtimeMessage(glance, telemetryNotifyPayload{})
		if data, err := json.Marshal(msg); err == nil {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-outbound:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
