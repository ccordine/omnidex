package api

import (
	"strings"
	"testing"
)

func TestRenderRecyclrTemplateHTMLRequiresTarget(t *testing.T) {
	assertPanicsWith(t, "recyclr template target is required", func() {
		renderRecyclrTemplateHTML(" ", "<p>body</p>", "innerHTML")
	})
}

func TestRenderRecyclrTemplateHTMLRejectsUnsupportedLocation(t *testing.T) {
	assertPanicsWith(t, `unsupported recyclr template location: "append"`, func() {
		renderRecyclrTemplateHTML("timeline", "<p>body</p>", "append")
	})
}

func TestRenderRecyclrTemplateHTMLEscapesAttributes(t *testing.T) {
	html := renderRecyclrTemplateHTML(`bad"target`, "<p>body</p>", "innerHTML")
	if !strings.Contains(html, `data-recyclr-target="bad&#34;target"`) {
		t.Fatalf("expected escaped target attribute, got %q", html)
	}
}

func TestScrumBoardFragmentPayloadRejectsEveryMalformedTypedField(t *testing.T) {
	base := validScrumFragmentPayload()
	for _, field := range []string{
		"project_id", "board", "all_columns", "cards_by_col", "column_counts", "play_queue", "auto_work",
		"auto_work_complete", "flow_summary", "visible_column", "card_offset", "card_has_more",
	} {
		t.Run(field, func(t *testing.T) {
			payload := make(map[string]any, len(base))
			for key, value := range base {
				payload[key] = value
			}
			payload[field] = "wrong type"
			if field == "visible_column" {
				payload[field] = 42
			}
			if err := scrumBoardFragmentsForPayload(payload, ScrumBoard{Columns: append([]string(nil), scrumColumns...)}); err == nil ||
				!strings.Contains(err.Error(), field) {
				t.Fatalf("field %s error=%v", field, err)
			}
		})
	}
}

func TestScrumBoardFragmentPayloadRequiresExactInventory(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		payload := validScrumFragmentPayload()
		if err := scrumBoardFragmentsForPayload(payload, ScrumBoard{Columns: append([]string(nil), scrumColumns...)}); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["html"].(scrumBoardFragments); !ok {
			t.Fatalf("valid exact payload did not receive typed fragments: %T", payload["html"])
		}
	})
	t.Run("missing", func(t *testing.T) {
		payload := validScrumFragmentPayload()
		delete(payload, "flow_summary")
		if err := scrumBoardFragmentsForPayload(payload, ScrumBoard{}); err == nil || !strings.Contains(err.Error(), "exactly") {
			t.Fatalf("missing field error=%v", err)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		payload := validScrumFragmentPayload()
		payload["client_fallback"] = true
		if err := scrumBoardFragmentsForPayload(payload, ScrumBoard{}); err == nil || !strings.Contains(err.Error(), "exactly") {
			t.Fatalf("unknown field error=%v", err)
		}
	})
}

func TestScrumBoardFragmentsRejectMalformedPlayQueueInsteadOfInventingIdleState(t *testing.T) {
	for name, playQueue := range map[string]map[string]any{
		"missing field": {
			"running_card_id": "", "queued_count": 0, "queued_card_ids": []string{},
		},
		"wrong running id": {
			"running_card_id": 42, "queued_count": 0, "queued_card_ids": []string{}, "queued_has_more": false,
		},
		"contradictory count": {
			"running_card_id": "", "queued_count": 1, "queued_card_ids": []string{}, "queued_has_more": false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			payload := validScrumFragmentPayload()
			payload["play_queue"] = playQueue
			if err := scrumBoardFragmentsForPayload(payload, ScrumBoard{Columns: append([]string(nil), scrumColumns...)}); err == nil {
				t.Fatal("malformed play queue unexpectedly rendered as idle")
			}
		})
	}
}

func TestScrumFragmentSourceHasNoIgnoredPayloadAssertionsOrPlayQueueFallback(t *testing.T) {
	source := readAPISource(t, "scrum_fragments.go")
	for _, forbidden := range []string{
		`board, _ := payload["board"]`,
		`value, _ := playQueue[key].(string)`,
		`columns = []string{"assigned"}`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum fragment rendering retains ignored/defaulted authority %q", forbidden)
		}
	}
}

func validScrumFragmentPayload() map[string]any {
	board := ScrumBoard{ID: "project_1", Columns: []string{"assigned"}, Cards: []ScrumCard{}}
	autoWork := defaultScrumAutoWorkConfig()
	return map[string]any{
		"board":         board,
		"cards_by_col":  map[string][]ScrumCard{"assigned": {}},
		"column_counts": map[string]int{"assigned": 0},
		"play_queue": map[string]any{
			"running_card_id": "", "queued_count": 0,
			"queued_card_ids": []string{}, "queued_has_more": false,
		},
		"auto_work": autoWork, "auto_work_complete": false,
		"flow_summary": ScrumFlowProjectSummary{}, "visible_column": "assigned",
		"card_offset": 0, "card_has_more": false,
		"project_id": int64(1), "all_columns": append([]string(nil), scrumColumns...),
	}
}

func assertPanicsWith(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		got := recovered.(string)
		if !strings.Contains(got, want) {
			t.Fatalf("expected panic containing %q, got %q", want, got)
		}
	}()
	fn()
}
