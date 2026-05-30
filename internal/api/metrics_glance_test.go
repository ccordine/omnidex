package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestRenderMetricsNavBadgesHTML(t *testing.T) {
	html := renderMetricsNavBadgesHTML(queue.TelemetryGlanceSummary{
		LiveRuns:     2,
		RecentErrors: 3,
	})
	if !strings.Contains(html, `data-recyclr-target="metrics-nav-badges"`) {
		t.Fatalf("expected recyclr target wrapper, got %q", html)
	}
	if !strings.Contains(html, ">2<") || !strings.Contains(html, ">3<") {
		t.Fatalf("expected live and error counts in html, got %q", html)
	}
}

func TestRenderMetricsNavBadgesHTMLEmpty(t *testing.T) {
	html := renderMetricsNavBadgesHTML(queue.TelemetryGlanceSummary{})
	if !strings.Contains(html, ">05<") {
		t.Fatalf("expected keyboard hint fallback, got %q", html)
	}
}

func TestBuildMetricsGlanceRealtimeMessageToast(t *testing.T) {
	s := &Server{}
	msg := s.buildMetricsGlanceRealtimeMessage(queue.TelemetryGlanceSummary{RecentErrors: 1}, telemetryNotifyPayload{
		EventType: "step_error",
		Message:   "tool failed",
	})
	if msg.Toast == "" {
		t.Fatal("expected toast for struggle event")
	}
	if msg.EventName != "metrics-glance" {
		t.Fatalf("eventName = %q", msg.EventName)
	}
}
