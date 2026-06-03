package api

import (
	"context"
	"net/url"
	"strings"
	"time"
)

const coreDependencyCheckTimeout = 1500 * time.Millisecond

type coreDependencyStatus struct {
	Status     string `json:"status"`
	Configured bool   `json:"configured"`
	Required   bool   `json:"required"`
	Reachable  bool   `json:"reachable"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Target     string `json:"target,omitempty"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

func (s *Server) collectCoreDependencies(ctx context.Context) map[string]coreDependencyStatus {
	return map[string]coreDependencyStatus{
		"postgres":    s.checkPostgresDependency(ctx),
		"redis":       s.checkRedisDependency(ctx),
		"host_bridge": s.checkHostBridgeDependency(ctx),
	}
}

func coreHealthStatus(dependencies map[string]coreDependencyStatus) string {
	for _, dependency := range dependencies {
		if dependency.Status == "error" {
			return "degraded"
		}
	}
	return "ok"
}

func (s *Server) checkPostgresDependency(ctx context.Context) coreDependencyStatus {
	dependency := coreDependencyStatus{
		Configured: s.repo != nil,
		Required:   s.repo != nil,
	}
	if s.repo == nil {
		dependency.Status = "not_configured"
		dependency.Message = "Queue database is not configured; queue-backed features are disabled."
		return dependency
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, coreDependencyCheckTimeout)
	defer cancel()
	err := s.repo.Ping(checkCtx)
	dependency.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		dependency.Status = "error"
		dependency.Error = err.Error()
		dependency.Message = "Core cannot reach PostgreSQL."
		return dependency
	}
	dependency.Status = "ok"
	dependency.Reachable = true
	dependency.Message = "Core can reach PostgreSQL."
	return dependency
}

func (s *Server) checkRedisDependency(ctx context.Context) coreDependencyStatus {
	dependency := coreDependencyStatus{
		Configured: strings.TrimSpace(s.redisURL) != "",
		Required:   s.uiRedisRequired,
		Target:     redactedRedisTarget(s.redisURL),
	}
	if !dependency.Configured {
		if dependency.Required {
			dependency.Status = "error"
			dependency.Error = "redis url is required but not configured"
			dependency.Message = "UI Redis is required but REDIS_URL is not configured."
			return dependency
		}
		dependency.Status = "not_configured"
		dependency.Message = "UI Redis is not configured; UI session state uses in-process storage."
		return dependency
	}
	if strings.TrimSpace(s.uiRedisInitError) != "" {
		dependency.Status = "error"
		dependency.Error = s.uiRedisInitError
		dependency.Message = "Core cannot initialize the Redis client."
		return dependency
	}
	if s.uiRedis == nil {
		dependency.Status = "error"
		dependency.Error = "redis client is not initialized"
		dependency.Message = "Core cannot initialize the Redis client."
		return dependency
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, coreDependencyCheckTimeout)
	defer cancel()
	err := s.uiRedis.Ping(checkCtx)
	dependency.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		dependency.Status = "error"
		dependency.Error = err.Error()
		dependency.Message = "Core cannot reach Redis."
		return dependency
	}
	dependency.Status = "ok"
	dependency.Reachable = true
	dependency.Message = "Core can reach Redis."
	return dependency
}

func (s *Server) checkHostBridgeDependency(ctx context.Context) coreDependencyStatus {
	configured := strings.TrimSpace(s.hostAgentURL) != ""
	dependency := coreDependencyStatus{
		Configured: configured,
		Required:   configured,
		Target:     strings.TrimSpace(s.hostAgentURL),
	}
	if !configured {
		dependency.Status = "not_configured"
		dependency.Message = "HOST_AGENT_URL is not configured; host bridge features are disabled."
		return dependency
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, coreDependencyCheckTimeout)
	defer cancel()
	status := s.collectHostBridgeStatusWithTimeout(checkCtx, coreDependencyCheckTimeout)
	dependency.LatencyMS = time.Since(started).Milliseconds()
	dependency.Target = firstNonEmpty(status.URL, dependency.Target)
	if status.Reachable {
		dependency.Status = "ok"
		dependency.Reachable = true
		dependency.Message = "Core can reach the host bridge."
		return dependency
	}
	dependency.Status = "error"
	dependency.Error = strings.TrimSpace(firstNonEmpty(status.Error, status.Message, "host bridge is unreachable"))
	dependency.Message = "Core cannot reach the host bridge."
	return dependency
}

func redactedRedisTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}
