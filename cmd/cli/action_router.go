package main

import (
	"strings"
	"unicode"
)

type chatCapability struct {
	Kind     string
	MinScore int
	Priority int
	Terms    []string
	Actions  []string
}

func matchChatCapabilityKind(
	input string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) string {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	tokens := tokenizeForCapabilityMatch(lower)

	caps := make([]chatCapability, 0, 5)
	if localMedia {
		caps = append(caps, chatCapability{
			Kind:     "local_media",
			MinScore: 3,
			Priority: 70,
			Terms: []string{
				"vlc", "media", "player", "playback", "episode", "show", "movie", "subtitle", "captions", "mpris", "playerctl",
			},
			Actions: []string{
				"play", "pause", "resume", "toggle", "skip", "next", "previous", "search", "find", "watch", "listen",
			},
		})
	}
	if localBrowser {
		caps = append(caps, chatCapability{
			Kind:     "local_browser",
			MinScore: 3,
			Priority: 60,
			Terms: []string{
				"browser", "tab", "tabs", "chrome", "chromium", "firefox", "edge", "brave", "opera", "vivaldi", "devtools", "console", "javascript",
			},
			Actions: []string{
				"show", "list", "read", "scan", "inspect", "check", "watch",
			},
		})
	}
	if localScreen {
		caps = append(caps, chatCapability{
			Kind:     "local_screen",
			MinScore: 3,
			Priority: 50,
			Terms: []string{
				"screen", "screenshot", "display", "monitor", "window", "ui", "ocr", "text", "image", "vision",
			},
			Actions: []string{
				"read", "describe", "summarize", "scan", "capture", "check", "look",
			},
		})
	}
	if localAudio {
		caps = append(caps, chatCapability{
			Kind:     "local_audio",
			MinScore: 3,
			Priority: 40,
			Terms: []string{
				"audio", "notes", "note", "microphone", "mic", "speaker", "transcript", "transcribe", "recording", "record",
			},
			Actions: []string{
				"start", "stop", "status", "search", "find", "capture", "listen", "transcribe",
			},
		})
	}
	if localShell {
		caps = append(caps, chatCapability{
			Kind:     "local_shell",
			MinScore: 2,
			Priority: 30,
			Terms: []string{
				"shell", "terminal", "command", "file", "directory", "folder", "path", "repo", "repository", "project", "docker", "test", "dependency",
				"ip", "port", "network", "vpn", "user", "username", "time", "date", "os", "kernel",
			},
			Actions: []string{
				"run", "execute", "create", "make", "rename", "move", "check", "show", "list", "install", "start", "stop",
			},
		})
	}

	if len(caps) == 0 {
		return ""
	}

	bestKind := ""
	bestScore := 0
	bestPriority := -1
	for _, cap := range caps {
		score := scoreCapability(lower, tokens, cap)
		if score < cap.MinScore {
			continue
		}
		if score > bestScore || (score == bestScore && cap.Priority > bestPriority) {
			bestKind = cap.Kind
			bestScore = score
			bestPriority = cap.Priority
		}
	}

	return bestKind
}

func scoreCapability(lower string, tokens map[string]struct{}, cap chatCapability) int {
	score := 0
	for _, term := range cap.Terms {
		score += scoreCapabilityPhrase(lower, tokens, term, 2)
	}
	for _, action := range cap.Actions {
		score += scoreCapabilityPhrase(lower, tokens, action, 1)
	}

	switch cap.Kind {
	case "local_shell":
		if _, ok := parseExplicitRunCommand(lower, lower); ok {
			score += 6
		}
		if strings.Contains(lower, "`") {
			score += 2
		}
	case "local_media":
		if containsAnyPhrase(lower, []string{"what just happened", "next episode", "what did they just say"}) {
			score += 4
		}
	case "local_browser":
		if containsAnyPhrase(lower, []string{"open tabs", "console logs", "javascript console"}) {
			score += 4
		}
	case "local_screen":
		if containsAnyPhrase(lower, []string{"what's on my screen", "what is on my screen", "read my screen"}) {
			score += 4
		}
	}
	return score
}

func scoreCapabilityPhrase(lower string, tokens map[string]struct{}, phrase string, weight int) int {
	term := strings.ToLower(strings.TrimSpace(phrase))
	if term == "" {
		return 0
	}
	if strings.Contains(term, " ") {
		if strings.Contains(lower, term) {
			return weight
		}
		return 0
	}
	norm := normalizeCapabilityToken(term)
	if norm == "" {
		return 0
	}
	if _, ok := tokens[norm]; ok {
		return weight
	}
	return 0
}

func tokenizeForCapabilityMatch(lower string) map[string]struct{} {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return r
		}
		return ' '
	}, lower)
	parts := strings.Fields(normalized)
	out := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		token := normalizeCapabilityToken(part)
		if token == "" {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}

func normalizeCapabilityToken(token string) string {
	value := strings.ToLower(strings.TrimSpace(token))
	if value == "" {
		return ""
	}
	if len(value) > 5 && strings.HasSuffix(value, "ing") {
		value = strings.TrimSuffix(value, "ing")
	}
	if len(value) > 4 && strings.HasSuffix(value, "ed") {
		value = strings.TrimSuffix(value, "ed")
	}
	if len(value) > 4 && strings.HasSuffix(value, "es") {
		value = strings.TrimSuffix(value, "es")
	}
	if len(value) > 4 && strings.HasSuffix(value, "s") {
		value = strings.TrimSuffix(value, "s")
	}
	return value
}
