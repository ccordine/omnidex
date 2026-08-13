package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	scrumFlowEventColumnMove       = "column_move"
	scrumFlowEventPlayStarted      = "play_started"
	scrumFlowEventPlayFinished     = "play_finished"
	scrumFlowEventPlayPaused       = "play_paused"
	scrumFlowEventConversation     = "conversation"
	scrumFlowEventReviewGatePassed = "review_gate_passed"
	scrumFlowEventReviewGateFailed = "review_gate_failed"
)

type ScrumFlowMetrics struct {
	AssignedReturns   int      `json:"assigned_returns"`
	ReviewBounces     int      `json:"review_bounces"`
	RegressionCount   int      `json:"regression_count"`
	PlayRuns          int      `json:"play_runs"`
	ChannelMessages   int      `json:"channel_messages"`
	ConversationChars int      `json:"conversation_chars"`
	IncompleteScore   int      `json:"incomplete_score"`
	CompletionStatus  string   `json:"completion_status"`
	Signals           []string `json:"signals"`
	LastPlayOutcome   string   `json:"last_play_outcome,omitempty"`
	ReviewGate        string   `json:"review_gate,omitempty"`
	Column            string   `json:"column,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

type ScrumFlowProjectSummary struct {
	TotalCards           int `json:"total_cards"`
	LikelyIncomplete     int `json:"likely_incomplete"`
	Uncertain            int `json:"uncertain"`
	LikelyComplete       int `json:"likely_complete"`
	AssignedReturnsTotal int `json:"assigned_returns_total"`
	LongConversations    int `json:"long_conversations"`
}

var scrumColumnRank = map[string]int{
	"backlog":     0,
	"ready":       1,
	"assigned":    2,
	"in_progress": 3,
	"review":      4,
	"blocked":     4,
	"done":        5,
}

func scrumColumnRankValue(column string) int {
	if rank, ok := scrumColumnRank[normalizeScrumColumn(column)]; ok {
		return rank
	}
	return 0
}

func isScrumRegressionToAssigned(fromColumn, toColumn string) bool {
	to := normalizeScrumColumn(toColumn)
	from := normalizeScrumColumn(fromColumn)
	if to != "assigned" || from == "" || from == to {
		return false
	}
	return scrumColumnRankValue(from) > scrumColumnRankValue("assigned")
}

func isScrumReviewBounce(fromColumn, toColumn string) bool {
	from := normalizeScrumColumn(fromColumn)
	to := normalizeScrumColumn(toColumn)
	if from != "review" {
		return false
	}
	return to == "assigned" || to == "in_progress"
}

func isScrumColumnRegression(fromColumn, toColumn string) bool {
	from := normalizeScrumColumn(fromColumn)
	to := normalizeScrumColumn(toColumn)
	if from == "" || to == "" || from == to {
		return false
	}
	return scrumColumnRankValue(to) < scrumColumnRankValue(from)
}

func conversationStats(card ScrumCard) (channelMessages, totalChars int) {
	for _, msg := range card.Chat {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		channelMessages++
		totalChars += len(msg.Content)
	}
	return channelMessages, totalChars
}

func checklistIncomplete(card ScrumCard) bool {
	if len(card.Checklist) == 0 {
		return false
	}
	for _, item := range card.Checklist {
		if !item.Done {
			return true
		}
	}
	return false
}

func computeScrumFlowMetrics(card ScrumCard, events []queue.ScrumFlowEvent) ScrumFlowMetrics {
	metrics := ScrumFlowMetrics{
		Column:           normalizeScrumColumn(card.Column),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
		CompletionStatus: "uncertain",
		Signals:          []string{},
	}
	metrics.ChannelMessages, metrics.ConversationChars = conversationStats(card)

	for _, event := range events {
		switch event.EventType {
		case scrumFlowEventColumnMove:
			if isScrumRegressionToAssigned(event.FromColumn, event.ToColumn) {
				metrics.AssignedReturns++
				metrics.RegressionCount++
			} else if isScrumReviewBounce(event.FromColumn, event.ToColumn) {
				metrics.ReviewBounces++
				metrics.RegressionCount++
			} else if isScrumColumnRegression(event.FromColumn, event.ToColumn) {
				metrics.RegressionCount++
			}
		case scrumFlowEventPlayStarted:
			metrics.PlayRuns++
		case scrumFlowEventPlayFinished:
			var payload map[string]any
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				log.Printf("scrum flow event payload rejected event_id=%d card=%q: %v", event.ID, event.CardID, err)
				continue
			}
			if outcome, ok := payload["outcome"].(string); ok {
				metrics.LastPlayOutcome = strings.TrimSpace(outcome)
			}
		case scrumFlowEventReviewGatePassed:
			metrics.ReviewGate = "passed"
		case scrumFlowEventReviewGateFailed:
			metrics.ReviewGate = "failed"
		}
	}

	score := 0
	addSignal := func(text string, weight int) {
		if weight <= 0 {
			return
		}
		score += weight
		metrics.Signals = append(metrics.Signals, text)
	}

	if metrics.AssignedReturns > 0 {
		addSignal(fmt.Sprintf("returned to assigned %d time(s) after review or later", metrics.AssignedReturns), metrics.AssignedReturns*25)
	}
	if metrics.ReviewBounces > 0 {
		addSignal(fmt.Sprintf("bounced out of review %d time(s)", metrics.ReviewBounces), metrics.ReviewBounces*20)
	}
	if metrics.RegressionCount > metrics.AssignedReturns+metrics.ReviewBounces {
		extra := metrics.RegressionCount - metrics.AssignedReturns - metrics.ReviewBounces
		addSignal(fmt.Sprintf("%d other column regression(s)", extra), extra*10)
	}
	if metrics.PlayRuns > 2 {
		addSignal(fmt.Sprintf("played %d times", metrics.PlayRuns), (metrics.PlayRuns-2)*10)
	}
	totalMessages := metrics.ChannelMessages
	if totalMessages >= 30 && metrics.Column != "done" {
		addSignal(fmt.Sprintf("long conversation (%d messages) without reaching done", totalMessages), 15)
	}
	if metrics.ConversationChars >= 10000 && metrics.Column != "done" {
		addSignal(fmt.Sprintf("heavy discussion (~%dk chars) still open", metrics.ConversationChars/1000), 10)
	}
	if metrics.Column == "blocked" {
		addSignal("currently blocked", 20)
	}
	if checklistIncomplete(card) && (metrics.Column == "review" || metrics.Column == "done") {
		addSignal("checklist still incomplete in review/done", 15)
	}
	if metrics.LastPlayOutcome == "failed" || metrics.LastPlayOutcome == "blocked" {
		addSignal("last play outcome: "+metrics.LastPlayOutcome, 15)
	}

	metrics.IncompleteScore = score
	switch {
	case score >= 50:
		metrics.CompletionStatus = "likely_incomplete"
	case score <= 15 && metrics.Column == "done" && metrics.AssignedReturns == 0:
		metrics.CompletionStatus = "likely_complete"
	default:
		metrics.CompletionStatus = "uncertain"
	}
	return metrics
}

func summarizeScrumFlowMetrics(cards []ScrumCard) ScrumFlowProjectSummary {
	summary := ScrumFlowProjectSummary{TotalCards: len(cards)}
	for _, card := range cards {
		metrics := parseScrumFlowMetrics(card.FlowMetrics)
		summary.AssignedReturnsTotal += metrics.AssignedReturns
		if metrics.ChannelMessages >= 30 {
			summary.LongConversations++
		}
		switch metrics.CompletionStatus {
		case "likely_incomplete":
			summary.LikelyIncomplete++
		case "likely_complete":
			summary.LikelyComplete++
		default:
			summary.Uncertain++
		}
	}
	return summary
}

func parseScrumFlowMetrics(raw json.RawMessage) ScrumFlowMetrics {
	out := ScrumFlowMetrics{CompletionStatus: "uncertain", Signals: []string{}}
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		log.Printf("scrum flow metrics payload rejected bytes=%d: %v", len(raw), err)
		return out
	}
	if out.Signals == nil {
		out.Signals = []string{}
	}
	return out
}
