// Package inferenceapi serves one bounded semantic question without creating
// channels, jobs, memory, transcript records, or model-owned actions.
package inferenceapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Provider interface {
	Predict(context.Context, Request) (string, error)
	Check(context.Context) error
}

type Request struct {
	Question        string `json:"question"`
	Context         string `json:"context"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

type Result struct {
	Text       string `json:"text"`
	Model      string `json:"model"`
	DurationMS int64  `json:"duration_ms"`
}

type Server struct {
	client       Provider
	model, token string
	timeout      time.Duration
	slots        chan struct{}
}

func New(client Provider, model, token string, timeout time.Duration) (*Server, error) {
	if client == nil || strings.TrimSpace(model) == "" || len(token) < 32 || timeout <= 0 || timeout > time.Minute {
		return nil, fmt.Errorf("inference requires a generator, model, integration token, and timeout up to one minute")
	}
	for _, char := range token {
		if char < 33 || char > 126 {
			return nil, fmt.Errorf("integration token must contain visible ASCII characters")
		}
	}
	return &Server{client: client, model: model, token: token, timeout: timeout, slots: make(chan struct{}, 2)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"status": "ready", "model": s.model, "retains_history": false})
	})
	mux.HandleFunc("POST /v1/integrations/infer", s.infer)
	return mux
}

func (s *Server) infer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	provided := sha256.Sum256([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
	wanted := sha256.Sum256([]byte(s.token))
	if len(r.Header.Values("Authorization")) != 1 || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare(provided[:], wanted[:]) != 1 {
		respond(w, 401, map[string]string{"error": "integration API authentication failed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 24*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input Request
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || validate(input) != nil {
		respond(w, 400, map[string]string{"error": "provide one bounded question, context, and 1–1024 output tokens"})
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		respond(w, 429, map[string]string{"error": "local inference is busy"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	started := time.Now()
	result, err := s.client.Predict(ctx, input)
	if err != nil || strings.TrimSpace(result) == "" || len(result) > 16384 || !utf8.ValidString(result) || strings.ContainsRune(result, 0) {
		// Provider errors can contain input/output; neither log nor return them.
		respond(w, 502, map[string]string{"error": "local model did not return a complete bounded result"})
		return
	}
	respond(w, 200, Result{Text: strings.TrimSpace(result), Model: s.model, DurationMS: time.Since(started).Milliseconds()})
}

func validate(input Request) error {
	if strings.TrimSpace(input.Question) == "" || len(input.Question) > 4096 || len(input.Context) > 12000 || !utf8.ValidString(input.Question) || !utf8.ValidString(input.Context) || strings.ContainsRune(input.Question+input.Context, 0) || input.MaxOutputTokens < 1 || input.MaxOutputTokens > 1024 {
		return fmt.Errorf("invalid semantic request")
	}
	return nil
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
