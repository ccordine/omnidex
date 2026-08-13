package api

import "net/http"

func (s *Server) handleChatMetricsComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot, err := s.collectChatMetrics(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	bundle, err := renderChatMetricsBundle(snapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, struct {
		HTML chatComponentHTML `json:"html"`
	}{HTML: chatComponentHTML{Bundle: bundle}})
}

func (s *Server) collectChatMetrics(r *http.Request) (chatMetricsSnapshot, error) {
	var snapshot chatMetricsSnapshot
	var err error
	if snapshot.Live, err = s.repo.TelemetryLive(r.Context()); err != nil {
		return snapshot, err
	}
	if snapshot.Models, err = s.repo.TelemetryModelSummaries(r.Context()); err != nil {
		return snapshot, err
	}
	if snapshot.Playbooks, err = s.repo.TelemetryPlaybookSummaries(r.Context()); err != nil {
		return snapshot, err
	}
	if snapshot.Benchmarks, err = s.repo.TelemetryBenchmarkSummaries(r.Context()); err != nil {
		return snapshot, err
	}
	if snapshot.Shrink, err = s.repo.ContextShrinkMetrics(r.Context(), "", 1); err != nil {
		return snapshot, err
	}
	if snapshot.Usage, err = s.repo.LLMContextUsageMetrics(r.Context(), "", 1); err != nil {
		return snapshot, err
	}
	if snapshot.Operations, err = s.repo.OperationsMetrics(r.Context()); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}
