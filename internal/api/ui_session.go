package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const uiSessionCookieName = "omni_ui_session"

type uiSessionResponse struct {
	SessionID string         `json:"session_id"`
	State     map[string]any `json:"state"`
	Locale    uiLocale       `json:"locale"`
	Source    string         `json:"source"`
	TTLMS     int64          `json:"ttl_ms"`
}

func (s *Server) handleUISession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.readUISession(w, r)
	case http.MethodPatch, http.MethodPost:
		s.updateUISession(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) readUISession(w http.ResponseWriter, r *http.Request) {
	sessionID := s.ensureUISessionCookie(w, r)
	state, source, err := s.loadUIState(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := mergeUIQueryState(state, r); err != nil {
		logUILocaleRejection(sessionID, "session_read", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	locale, err := ensureUIStateLocale(state, r)
	if err != nil {
		logUILocaleRejection(sessionID, "session_read", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.persistUIState(r.Context(), sessionID, state); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	setUILocaleResponseHeaders(w, locale)
	writeJSON(w, http.StatusOK, uiSessionResponse{
		SessionID: sessionID,
		State:     state,
		Locale:    locale,
		Source:    source,
		TTLMS:     s.uiSessionTTL.Milliseconds(),
	})
}

func (s *Server) updateUISession(w http.ResponseWriter, r *http.Request) {
	sessionID := s.ensureUISessionCookie(w, r)
	state, _, err := s.loadUIState(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	var req struct {
		State map[string]any `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	previousLocale, _ := state["locale"].(string)
	if err := applyUIStatePatch(state, req.State); err != nil {
		logUILocaleRejection(sessionID, "session_patch", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := mergeUIQueryState(state, r); err != nil {
		logUILocaleRejection(sessionID, "session_patch", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	locale, err := ensureUIStateLocale(state, r)
	if err != nil {
		logUILocaleRejection(sessionID, "session_patch", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	source, err := s.persistUIState(r.Context(), sessionID, state)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	logUILocaleTransition(sessionID, previousLocale, locale, source)
	setUILocaleResponseHeaders(w, locale)
	writeJSON(w, http.StatusOK, uiSessionResponse{
		SessionID: sessionID,
		State:     state,
		Locale:    locale,
		Source:    source,
		TTLMS:     s.uiSessionTTL.Milliseconds(),
	})
}

func (s *Server) ensureUISessionCookie(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(uiSessionCookieName); err == nil {
		if id := normalizeUISessionID(cookie.Value); id != "" {
			return id
		}
	}
	sessionID := newUISessionID()
	http.SetCookie(w, &http.Cookie{
		Name:     uiSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(s.uiSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return sessionID
}

func (s *Server) loadUIState(ctx context.Context, sessionID string) (map[string]any, string, error) {
	key := uiSessionRedisKey(sessionID)
	if s.uiRedis != nil {
		raw, ok, err := s.uiRedis.Get(ctx, key)
		if err == nil && ok {
			state, err := decodeUIState([]byte(raw))
			if err == nil {
				return state, "redis", nil
			}
		}
		if err != nil && s.uiRedisRequired {
			return nil, "", fmt.Errorf("redis ui session unavailable: %w", err)
		}
	}
	if s.repo != nil {
		record, ok, err := s.repo.GetUISession(ctx, sessionID)
		if err != nil {
			return nil, "", err
		}
		if ok {
			state, err := decodeUIState(record.State)
			if err != nil {
				return map[string]any{}, "pgsql", nil
			}
			if s.uiRedis != nil {
				_ = s.uiRedis.SetEX(ctx, key, string(record.State), s.uiSessionTTL)
			}
			return state, "pgsql", nil
		}
	}
	return map[string]any{}, "new", nil
}

func (s *Server) persistUIState(ctx context.Context, sessionID string, state map[string]any) (string, error) {
	state = sanitizeUIState(state)
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	if s.uiRedis != nil {
		if err := s.uiRedis.SetEX(ctx, uiSessionRedisKey(sessionID), string(raw), s.uiSessionTTL); err == nil {
			if s.repo != nil {
				_, _ = s.repo.UpsertUISession(ctx, sessionID, raw, s.uiSessionTTL)
			}
			return "redis", nil
		} else if s.uiRedisRequired {
			return "", fmt.Errorf("redis ui session unavailable: %w", err)
		}
	}
	if s.repo != nil {
		if _, err := s.repo.UpsertUISession(ctx, sessionID, raw, s.uiSessionTTL); err != nil {
			return "", err
		}
		return "pgsql", nil
	}
	return "memory", nil
}

func decodeUIState(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return sanitizeUIState(state), nil
}

func sanitizeUIState(state map[string]any) map[string]any {
	clean := make(map[string]any)
	for key, value := range state {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		switch v := value.(type) {
		case string:
			clean[key] = strings.TrimSpace(v)
		case float64, bool:
			clean[key] = v
		case nil:
			continue
		default:
			if raw, err := json.Marshal(v); err == nil && len(raw) <= 4096 {
				clean[key] = v
			}
		}
	}
	return clean
}

func normalizeUISessionID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 16 || len(value) > 96 {
		return ""
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return ""
	}
	return value
}

func newUISessionID() string {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(buf[:])
}

func uiSessionRedisKey(sessionID string) string {
	return "omni:ui:session:" + sessionID
}
