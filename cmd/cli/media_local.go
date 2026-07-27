package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const mediaProbeTimeout = 3 * time.Second
const mediaScanFileLimit = 5000
const mediaScanMaxDepth = 2

var videoExtensions = map[string]struct{}{
	".avi":  {},
	".flv":  {},
	".m4v":  {},
	".mkv":  {},
	".mov":  {},
	".mp4":  {},
	".mpeg": {},
	".mpg":  {},
	".ts":   {},
	".webm": {},
	".wmv":  {},
}

type nextEpisodeIntent struct {
	ShowHint string
}

type playbackControlIntent struct {
	Action string
}

type playbackContextIntent struct {
	Query       string
	BeforeLines int
	AfterLines  int
}

type mediaPlayerState struct {
	Player            string
	Status            string
	Title             string
	SourceURL         string
	CurrentPath       string
	PositionSeconds   float64
	VLCProcessRunning bool
}

func tryHandleLocalMediaCommand(input string) (bool, string) {
	if controlIntent, ok := parsePlaybackControlIntent(input); ok {
		permissionKey := permissionKeyMediaControl
		reason := "Allow local media playback control and opening next episode files."
		if strings.EqualFold(strings.TrimSpace(controlIntent.Action), "status") {
			permissionKey = permissionKeyMediaRead
			reason = "Allow reading local player metadata (status/title/path)."
		}
		if err := ensureLocalPermission(permissionKey, reason); err != nil {
			return true, "Local media action blocked: " + err.Error()
		}
		outcome, err := controlMediaPlayback(controlIntent)
		if err != nil {
			return true, "Local media action failed: " + err.Error()
		}
		return true, outcome
	}

	if nextIntent, ok := parseNextEpisodeIntent(input); ok {
		if err := ensureLocalPermission(permissionKeyMediaControl, "Allow local media playback control and opening next episode files."); err != nil {
			return true, "Local media action blocked: " + err.Error()
		}
		outcome, err := playNextEpisode(nextIntent)
		if err != nil {
			return true, "Local media action failed: " + err.Error()
		}
		return true, outcome
	}

	if contextIntent, ok := parsePlaybackContextIntent(input); ok {
		if err := ensureLocalPermission(permissionKeyMediaRead, "Allow reading local player metadata and subtitle timestamps for playback context."); err != nil {
			return true, "Local media action blocked: " + err.Error()
		}
		outcome, err := describeCurrentPlaybackMoment(contextIntent)
		if err != nil {
			return true, "Local media action failed: " + err.Error()
		}
		return true, outcome
	}

	return false, ""
}

func parsePlaybackControlIntent(input string) (playbackControlIntent, bool) {
	clean := strings.TrimSpace(input)
	lower := strings.ToLower(clean)
	if lower == "" {
		return playbackControlIntent{}, false
	}
	if strings.Contains(lower, "next episode") || strings.Contains(lower, "next ep") {
		return playbackControlIntent{}, false
	}

	switch {
	case isPlaybackStatusIntent(lower):
		return playbackControlIntent{Action: "status"}, true
	case containsAnyPhrase(lower, []string{
		"press play",
		"hit play",
		"resume",
		"unpause",
		"play it",
		"play my vlc",
		"start vlc",
		"continue playback",
	}) || (strings.Contains(lower, "play") && strings.Contains(lower, "vlc") && !isPlaybackStatusIntent(lower)):
		return playbackControlIntent{Action: "play"}, true
	case containsAnyPhrase(lower, []string{
		"press pause",
		"hit pause",
		"pause playback",
		"pause vlc",
		"pause it",
	}) || (strings.Contains(lower, "pause") && strings.Contains(lower, "vlc")):
		return playbackControlIntent{Action: "pause"}, true
	case containsAnyPhrase(lower, []string{
		"toggle playback",
		"play pause",
		"toggle vlc",
	}):
		return playbackControlIntent{Action: "play-pause"}, true
	default:
		return playbackControlIntent{}, false
	}
}

func isPlaybackStatusIntent(lower string) bool {
	if containsAnyPhrase(lower, []string{
		"is vlc playing",
		"vlc status",
		"media status",
		"player status",
		"what's playing",
		"what is playing",
		"currently playing",
		"now playing",
		"what's on vlc",
		"what is on vlc",
	}) {
		return true
	}
	if strings.Contains(lower, "vlc") && strings.Contains(lower, "playing") {
		return containsAnyPhrase(lower, []string{"what", "which", "tell me", "show me", "currently", "right now"})
	}
	return false
}

func parseNextEpisodeIntent(input string) (nextEpisodeIntent, bool) {
	clean := strings.TrimSpace(input)
	lower := strings.ToLower(clean)
	if lower == "" {
		return nextEpisodeIntent{}, false
	}

	hasNextEpisode := strings.Contains(lower, "next episode") || strings.Contains(lower, "next ep")
	if !hasNextEpisode {
		return nextEpisodeIntent{}, false
	}

	playVerbs := []string{"play", "start", "watch", "continue"}
	verbFound := false
	for _, verb := range playVerbs {
		if strings.Contains(lower, verb) {
			verbFound = true
			break
		}
	}
	if !verbFound {
		return nextEpisodeIntent{}, false
	}

	showHint := ""
	for _, marker := range []string{" of ", " for "} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		showHint = strings.TrimSpace(clean[idx+len(marker):])
		break
	}

	showHint = strings.Trim(showHint, " \t\r\n\"'`.,!?;:()[]{}")
	return nextEpisodeIntent{ShowHint: showHint}, true
}

func controlMediaPlayback(intent playbackControlIntent) (string, error) {
	player, statusBefore, err := discoverControllablePlayer()
	if err != nil {
		return "", err
	}

	action := strings.TrimSpace(intent.Action)
	if action == "" {
		return "", errors.New("playback action is required")
	}

	statusAfter := statusBefore
	switch action {
	case "play", "pause", "play-pause":
		if _, err := playerctlValue(player, action); err != nil {
			return "", fmt.Errorf("unable to run playerctl action %q for %s: %w", action, player, err)
		}
		if status, err := playerctlValue(player, "status"); err == nil {
			statusAfter = strings.TrimSpace(status)
		}
	case "status":
		if status, err := playerctlValue(player, "status"); err == nil {
			statusAfter = strings.TrimSpace(status)
		}
	default:
		return "", fmt.Errorf("unsupported playback action %q", action)
	}

	headline := "Playback control executed."
	if action == "status" {
		headline = "Playback status read."
	}
	lines := []string{
		headline,
		"player=" + safeValue(player, "unknown"),
		"action=" + action,
		"status_before=" + safeValue(statusBefore, "unknown"),
		"status_after=" + safeValue(statusAfter, "unknown"),
	}
	if action == "status" {
		lines = appendPlayerMetadata(lines, player)
	}
	return strings.Join(lines, "\n"), nil
}

func appendPlayerMetadata(lines []string, player string) []string {
	metadataFields := []struct {
		key  string
		name string
	}{
		{key: "title", name: "xesam:title"},
		{key: "artist", name: "xesam:artist"},
		{key: "album", name: "xesam:album"},
		{key: "url", name: "xesam:url"},
	}
	for _, field := range metadataFields {
		value, err := playerctlValue(player, "metadata", field.name)
		if err != nil {
			continue
		}
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		lines = append(lines, field.key+"="+clean)
	}
	return lines
}
