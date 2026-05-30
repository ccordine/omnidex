package queue

import (
	"context"
	"strings"
)

type TelemetryGlanceSummary struct {
	LiveRuns        int    `json:"live_runs"`
	RecentErrors    int    `json:"recent_errors"`
	StruggleSignals int    `json:"struggle_signals"`
	FailedRuns      int    `json:"failed_runs"`
	Struggling      bool   `json:"struggling"`
	Tone            string `json:"tone"`
}

func IsTelemetryStruggleEvent(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	for _, candidate := range telemetryStruggleEventTypes {
		if eventType == candidate {
			return true
		}
	}
	return false
}

func (r *Repository) TelemetryGlance(ctx context.Context) (TelemetryGlanceSummary, error) {
	var liveRuns int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM omni_runs
		WHERE status IN ('running', 'pending')
	`).Scan(&liveRuns); err != nil {
		return TelemetryGlanceSummary{}, err
	}

	var recentErrors int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM omni_run_events
		WHERE created_at >= NOW() - INTERVAL '1 hour'
		  AND event_type = ANY($1)
	`, telemetryStruggleEventTypes).Scan(&recentErrors); err != nil {
		return TelemetryGlanceSummary{}, err
	}

	struggle, err := r.TelemetryStruggleSummary(ctx)
	if err != nil {
		return TelemetryGlanceSummary{}, err
	}
	struggleTotal := 0
	for _, item := range struggle.StruggleEvents {
		struggleTotal += item.Count
	}
	acceptTotal := 0
	for _, item := range struggle.AcceptEvents {
		acceptTotal += item.Count
	}

	counts, err := r.telemetryStatusCounts(ctx)
	if err != nil {
		return TelemetryGlanceSummary{}, err
	}
	failedRuns := counts["failed"]

	struggling := recentErrors > 0 || struggleTotal > acceptTotal || struggle.RecentStruggleRuns > 0
	tone := "ok"
	if struggling {
		tone = "warn"
	}
	if recentErrors > 0 || failedRuns > 0 {
		tone = "error"
	}

	return TelemetryGlanceSummary{
		LiveRuns:        liveRuns,
		RecentErrors:    recentErrors,
		StruggleSignals: struggleTotal,
		FailedRuns:      failedRuns,
		Struggling:      struggling,
		Tone:            tone,
	}, nil
}
