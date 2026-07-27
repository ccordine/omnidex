package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func parsePlaybackContextIntent(input string) (playbackContextIntent, bool) {
	clean := strings.TrimSpace(input)
	lower := strings.ToLower(clean)
	if lower == "" {
		return playbackContextIntent{}, false
	}

	isPlaybackQuestion := false
	strongPatterns := []string{
		"what did they just say",
		"what line was that",
		"what subtitle just played",
		"where in the episode are we",
		"where are we in the episode",
	}
	for _, pattern := range strongPatterns {
		if strings.Contains(lower, pattern) {
			isPlaybackQuestion = true
			break
		}
	}
	if !isPlaybackQuestion {
		if strings.Contains(lower, "just happened") {
			for _, token := range []string{"show", "episode", "movie", "scene", "vlc"} {
				if strings.Contains(lower, token) {
					isPlaybackQuestion = true
					break
				}
			}
		}
	}
	if !isPlaybackQuestion {
		return playbackContextIntent{}, false
	}

	query := extractAboutQuery(clean)
	return playbackContextIntent{
		Query:       query,
		BeforeLines: 2,
		AfterLines:  1,
	}, true
}

func playNextEpisode(intent nextEpisodeIntent) (string, error) {
	state, err := discoverMediaPlayerState()
	if err != nil {
		return "", err
	}
	if state.CurrentPath == "" {
		return "", errors.New("current media is not a local file; unable to resolve next episode")
	}

	nextPath, scannedCount, err := resolveNextEpisodePath(state.CurrentPath, intent.ShowHint)
	if err != nil {
		return "", err
	}

	method, err := openMediaWithVLC(state.Player, nextPath)
	if err != nil {
		return "", err
	}

	lines := []string{
		"Playing next episode.",
		"method=" + method,
		"player=" + safeValue(state.Player, "unknown"),
		"status=" + safeValue(state.Status, "unknown"),
		"current=" + state.CurrentPath,
		"next=" + nextPath,
		"files_considered=" + strconv.Itoa(scannedCount),
	}
	if strings.TrimSpace(intent.ShowHint) != "" {
		lines = append(lines, "show_hint="+intent.ShowHint)
	}
	return strings.Join(lines, "\n"), nil
}

func discoverControllablePlayer() (string, string, error) {
	vlcRunning := isProcessRunning("vlc")
	if _, err := exec.LookPath("playerctl"); err != nil {
		if vlcRunning {
			return "", "", errors.New("`playerctl` is required for VLC control (install `playerctl`)")
		}
		return "", "", errors.New("no active media player detected (`vlc` not running and `playerctl` not installed)")
	}

	players, err := listPlayerctlPlayers()
	if err != nil {
		return "", "", err
	}
	if len(players) == 0 {
		if vlcRunning {
			return "", "", errors.New("vlc is running but no MPRIS player is exposed; ensure VLC MPRIS interface is enabled")
		}
		return "", "", errors.New("no active MPRIS player found")
	}

	player, status := choosePlayer(players)
	if strings.TrimSpace(player) == "" {
		return "", "", errors.New("unable to select an active media player")
	}
	return player, status, nil
}

func discoverMediaPlayerState() (mediaPlayerState, error) {
	state := mediaPlayerState{
		VLCProcessRunning: isProcessRunning("vlc"),
	}

	if _, err := exec.LookPath("playerctl"); err != nil {
		if state.VLCProcessRunning {
			return state, errors.New("`playerctl` is required to read current VLC metadata (install `playerctl`)")
		}
		return state, errors.New("no active media player detected (`vlc` not running and `playerctl` not installed)")
	}

	players, err := listPlayerctlPlayers()
	if err != nil {
		return state, err
	}
	if len(players) == 0 {
		if state.VLCProcessRunning {
			return state, errors.New("vlc is running but no MPRIS player is exposed; ensure VLC MPRIS interface is enabled")
		}
		return state, errors.New("no active MPRIS player found")
	}

	player, status := choosePlayer(players)
	state.Player = player
	state.Status = status

	urlValue, err := playerctlValue(player, "metadata", "xesam:url")
	if err != nil {
		return state, fmt.Errorf("unable to read current media URL: %w", err)
	}
	state.SourceURL = strings.TrimSpace(urlValue)

	titleValue, _ := playerctlValue(player, "metadata", "xesam:title")
	state.Title = strings.TrimSpace(titleValue)
	positionValue, _ := playerctlValue(player, "position")
	state.PositionSeconds = parsePlayerPosition(positionValue)

	currentPath, err := pathFromMediaURL(state.SourceURL)
	if err != nil {
		return state, err
	}
	state.CurrentPath = currentPath
	return state, nil
}

func choosePlayer(players []string) (string, string) {
	type candidate struct {
		name   string
		status string
		score  int
	}

	best := candidate{}
	bestSet := false
	for _, player := range players {
		status, _ := playerctlValue(player, "status")
		c := candidate{name: player, status: strings.TrimSpace(status)}
		lowerName := strings.ToLower(player)
		lowerStatus := strings.ToLower(c.status)
		if strings.Contains(lowerName, "vlc") {
			c.score += 10
		}
		if lowerStatus == "playing" {
			c.score += 4
		}
		if lowerStatus == "paused" {
			c.score += 2
		}
		if !bestSet || c.score > best.score {
			best = c
			bestSet = true
		}
	}
	if bestSet {
		return best.name, best.status
	}
	return players[0], ""
}

func listPlayerctlPlayers() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, "playerctl", "-l")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		lower := strings.ToLower(text)
		if strings.Contains(lower, "no players found") || text == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("playerctl -l failed: %s", safeValue(text, err.Error()))
	}

	lines := strings.Split(text, "\n")
	seen := map[string]struct{}{}
	outPlayers := make([]string, 0, len(lines))
	for _, line := range lines {
		player := strings.TrimSpace(line)
		if player == "" {
			continue
		}
		if _, ok := seen[player]; ok {
			continue
		}
		seen[player] = struct{}{}
		outPlayers = append(outPlayers, player)
	}
	return outPlayers, nil
}

func playerctlValue(player string, args ...string) (string, error) {
	base := []string{"--player=" + player}
	base = append(base, args...)
	return runCommandOutput("playerctl", base...)
}

func runCommandOutput(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s command timed out", name)
	}
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}

func pathFromMediaURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("active player has no file metadata")
	}
	if strings.HasPrefix(value, "file://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid file URL %q", value)
		}
		decoded, err := url.PathUnescape(parsed.Path)
		if err != nil {
			return "", fmt.Errorf("invalid encoded file URL path %q", value)
		}
		if decoded == "" {
			return "", fmt.Errorf("missing file path in media URL %q", value)
		}
		return filepath.Clean(decoded), nil
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("current media source is non-file URI (%s)", value)
	}
	return filepath.Clean(value), nil
}

func resolveNextEpisodePath(currentPath, showHint string) (string, int, error) {
	currentAbs, err := filepath.Abs(currentPath)
	if err != nil {
		return "", 0, fmt.Errorf("cannot resolve current path %q: %w", currentPath, err)
	}
	currentAbs = filepath.Clean(currentAbs)

	candidates, err := discoverNeighborVideos(currentAbs)
	if err != nil {
		return "", 0, err
	}
	if len(candidates) == 0 {
		return "", 0, errors.New("no nearby video files found")
	}

	filtered := filterCandidatesByShowHint(candidates, showHint)
	if len(filtered) == 0 {
		filtered = candidates
	}

	next, err := pickNextPath(filtered, currentAbs)
	if err != nil {
		return "", len(filtered), err
	}
	return next, len(filtered), nil
}
