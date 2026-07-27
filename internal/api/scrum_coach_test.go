package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultScrumCoachConfigAutoScanOff(t *testing.T) {
	cfg := defaultScrumCoachConfig("test-model")
	if cfg.AutoScan {
		t.Fatal("expected auto_scan default false")
	}
	if !cfg.Enabled {
		t.Fatal("expected coach enabled by default")
	}
}

func TestParseScrumCoachConfigEmptyDefaultsAutoScanOff(t *testing.T) {
	cfg, err := parseScrumCoachConfig(nil, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoScan {
		t.Fatal("expected auto_scan false for empty config")
	}
}

func TestParseScrumCoachConfigExplicitAutoScan(t *testing.T) {
	raw := json.RawMessage(`{"auto_scan":true}`)
	cfg, err := parseScrumCoachConfig(raw, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoScan {
		t.Fatal("expected auto_scan true when set")
	}
}

func TestParseScrumCoachConfigRejectsMalformedOrUnknownState(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"enabled":"yes"}`),
		json.RawMessage(`{"model":""}`),
		json.RawMessage(`{"legacy_mode":true}`),
	} {
		if _, err := parseScrumCoachConfig(raw, "test-model"); err == nil {
			t.Fatalf("config %q must fail loudly", raw)
		}
	}
}

func TestParseCoachLLMResponseRequiresTypedJSON(t *testing.T) {
	response, err := parseCoachLLMResponse(`{"reply":"Clear next step","suggestions":[{"level":"tip","text":"Add a test"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if response.Reply != "Clear next step" || len(response.Suggestions) != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
	for _, raw := range []string{
		"plain text fallback",
		"```json\n{\"reply\":\"no\"}\n```",
		`{"reply":"ok","unknown":true}`,
		`{"reply":"","suggestions":[{"level":"urgent","text":"x"}]}`,
	} {
		if _, err := parseCoachLLMResponse(raw); err == nil {
			t.Fatalf("response %q must fail loudly", raw)
		}
	}
}

func TestNormalizeCoachModeRejectsUnknownCommandsAndModes(t *testing.T) {
	if mode, err := normalizeCoachMode("/plan split this", ""); err != nil || mode != "plan" {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	for _, input := range []struct{ message, mode string }{
		{message: "/mystery", mode: ""},
		{message: "hello", mode: "magic"},
	} {
		if _, err := normalizeCoachMode(input.message, input.mode); err == nil {
			t.Fatalf("mode=%q message=%q must fail", input.mode, input.message)
		}
	}
}

func TestScrumCoachSourceHasNoRawReplyOrMemoryFallback(t *testing.T) {
	source := readAPISource(t, "scrum_coach.go") + readAPISource(t, "scrum_coach_config.go")
	for _, forbidden := range []string{
		"out := ScrumCoachLLMResponse{Reply: raw}",
		"embedding = nil",
		"embedding, _ :=",
		"s.repo.AddMemoryChunk(r.Context()",
		"_ = s.mergeProjectTags",
		"_ = json.Unmarshal(project.Settings",
		`"qwen3:4b-thinking"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum coach contains hidden fallback %q", forbidden)
		}
	}
}
