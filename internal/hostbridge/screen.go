package hostbridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type ScreenMonitor struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Primary bool   `json:"primary"`
}

func (s *Server) handleScreenMonitors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	monitors, backend, err := listScreenMonitors()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"monitors":    monitors,
		"backend":     backend,
		"stream_path": "/v1/screen/mjpeg",
	})
}

func (s *Server) handleScreenMJPEG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	monitor := strings.TrimSpace(r.URL.Query().Get("monitor"))
	fps := clampInt(parseScreenInt(r.URL.Query().Get("fps"), 12), 1, 30)
	quality := clampInt(parseScreenInt(r.URL.Query().Get("quality"), 5), 2, 12)
	scalePct := clampInt(parseScreenInt(r.URL.Query().Get("scale"), 100), 25, 100)

	if err := streamScreenMJPEG(r.Context(), w, monitor, fps, quality, scalePct); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		writeError(w, http.StatusServiceUnavailable, err.Error())
	}
}

func parseScreenInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func pickScreenMonitor(monitors []ScreenMonitor, monitorID string) (ScreenMonitor, error) {
	if len(monitors) == 0 {
		return ScreenMonitor{}, fmt.Errorf("no monitors available")
	}
	want := strings.TrimSpace(monitorID)
	if want == "" {
		for _, monitor := range monitors {
			if monitor.Primary {
				return monitor, nil
			}
		}
		return monitors[0], nil
	}
	for _, monitor := range monitors {
		if monitor.ID == want || monitor.Name == want {
			return monitor, nil
		}
	}
	return ScreenMonitor{}, fmt.Errorf("monitor %q not found", want)
}
