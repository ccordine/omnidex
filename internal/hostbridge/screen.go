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

const (
	DefaultScreenMonitorPageSize = 20
	MaxScreenMonitorPageSize     = 100
)

type ScreenMonitorPageRequest struct {
	Limit  int
	Offset int
}

type ScreenMonitorPage struct {
	Monitors       []ScreenMonitor `json:"monitors"`
	Backend        string          `json:"backend"`
	StreamPath     string          `json:"stream_path"`
	Limit          int             `json:"limit"`
	Offset         int             `json:"offset"`
	HasPrevious    bool            `json:"has_previous"`
	PreviousOffset int             `json:"previous_offset"`
	HasMore        bool            `json:"has_more"`
	NextOffset     int             `json:"next_offset"`
}

type screenMonitorScan func(func(ScreenMonitor) bool) (complete bool, count int, err error)

func (s *Server) handleScreenMonitors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	request, err := screenMonitorPageRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := listScreenMonitorPage(request)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func screenMonitorPageRequest(r *http.Request) (ScreenMonitorPageRequest, error) {
	limit, err := exactScreenPageInteger(r, "limit", DefaultScreenMonitorPageSize, 1, MaxScreenMonitorPageSize)
	if err != nil {
		return ScreenMonitorPageRequest{}, err
	}
	offset, err := exactScreenPageInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		return ScreenMonitorPageRequest{}, err
	}
	return ScreenMonitorPageRequest{Limit: limit, Offset: offset}, nil
}

func exactScreenPageInteger(r *http.Request, key string, fallback, minimum, maximum int) (int, error) {
	values, exists := r.URL.Query()[key]
	if !exists {
		return fallback, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, fmt.Errorf("%s must be one canonical integer", key)
	}
	value, err := strconv.Atoi(values[0])
	if err != nil || strconv.Itoa(value) != values[0] || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s is outside its accepted integer range", key)
	}
	return value, nil
}

func collectScreenMonitorPage(request ScreenMonitorPageRequest, backend string, scan screenMonitorScan) (ScreenMonitorPage, bool, error) {
	if request.Limit < 1 || request.Limit > MaxScreenMonitorPageSize {
		return ScreenMonitorPage{}, false, fmt.Errorf("monitor limit must be between 1 and %d", MaxScreenMonitorPageSize)
	}
	if request.Offset < 0 {
		return ScreenMonitorPage{}, false, fmt.Errorf("monitor offset must be non-negative")
	}
	retained := make([]ScreenMonitor, 0, request.Limit+1)
	seen := 0
	_, count, err := scan(func(monitor ScreenMonitor) bool {
		if seen < request.Offset {
			seen++
			return true
		}
		seen++
		retained = append(retained, monitor)
		return len(retained) <= request.Limit
	})
	if err != nil {
		return ScreenMonitorPage{}, false, err
	}
	found := count > 0
	hasMore := len(retained) > request.Limit
	if hasMore {
		retained = retained[:request.Limit]
	}
	previous := request.Offset - request.Limit
	if previous < 0 {
		previous = 0
	}
	next := 0
	if hasMore {
		next = request.Offset + len(retained)
	}
	return ScreenMonitorPage{
		Monitors: retained, Backend: backend, StreamPath: "/v1/screen/mjpeg",
		Limit: request.Limit, Offset: request.Offset,
		HasPrevious: request.Offset > 0, PreviousOffset: previous,
		HasMore: hasMore, NextOffset: next,
	}, found, nil
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
