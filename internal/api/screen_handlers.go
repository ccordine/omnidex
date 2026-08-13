package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (s *Server) handleHostScreenMonitors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.hostBridgeClient() == nil {
		writeError(w, http.StatusServiceUnavailable, "host bridge unavailable: run `omni host serve` on the host and set HOST_AGENT_URL")
		return
	}
	if _, err := s.resolveProjectID(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pageRequest, err := screenMonitorPageRequest(r, hostbridge.DefaultScreenMonitorPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	payload, err := s.hostBridgeClient().ScreenMonitors(ctx, pageRequest)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleHostScreenMJPEG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.hostBridgeClient() == nil {
		writeError(w, http.StatusServiceUnavailable, "host bridge unavailable: run `omni host serve` on the host and set HOST_AGENT_URL")
		return
	}
	if _, err := s.resolveProjectID(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	bridgeURL, err := buildBridgeScreenStreamURL(s.hostAgentURL, r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bridgeURL, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if token := strings.TrimSpace(s.hostAgentToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		writeError(w, http.StatusBadGateway, message)
		return
	}

	for key, values := range resp.Header {
		lower := strings.ToLower(key)
		if lower == "content-type" || lower == "cache-control" || lower == "pragma" || strings.HasPrefix(lower, "x-omni-screen-") {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func buildBridgeScreenStreamURL(base string, query url.Values) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported host bridge URL scheme")
	}
	parsed.Path = "/v1/screen/mjpeg"
	params := url.Values{}
	for _, key := range []string{"monitor", "fps", "quality", "scale"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			params.Set(key, value)
		}
	}
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}
