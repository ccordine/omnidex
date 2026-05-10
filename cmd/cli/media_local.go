package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gryph/omnidex/internal/media"
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

func describeCurrentPlaybackMoment(intent playbackContextIntent) (string, error) {
	state, err := discoverMediaPlayerState()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state.CurrentPath) == "" {
		return "", errors.New("current media is not a local file")
	}
	if state.PositionSeconds <= 0 {
		return "", errors.New("unable to read current playback timestamp from player")
	}

	subtitlePath, ep := resolveCurrentSubtitle(state.CurrentPath)
	if subtitlePath == "" {
		return "", fmt.Errorf("no subtitle file found near %s", state.CurrentPath)
	}

	lines, err := media.ParseSubtitleLines(subtitlePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse subtitles: %w", err)
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("subtitle file has no dialogue lines: %s", subtitlePath)
	}

	positionMS := int64(state.PositionSeconds * 1000)
	idx, mode, deltaMS := selectSubtitleAnchorLine(lines, positionMS, intent.Query)
	if idx < 0 || idx >= len(lines) {
		return "", errors.New("no subtitle line matched current timestamp")
	}

	before, after := subtitleContextWindow(lines, idx, intent.BeforeLines, intent.AfterLines)
	center := lines[idx]

	response := []string{
		"Playback context from subtitles.",
		"player=" + safeValue(state.Player, "unknown"),
		"status=" + safeValue(state.Status, "unknown"),
		"title=" + safeValue(state.Title, filepath.Base(state.CurrentPath)),
		"video=" + state.CurrentPath,
		"subtitle=" + subtitlePath,
		"position=" + formatPlaybackSeconds(state.PositionSeconds),
		"match_mode=" + mode,
	}
	if deltaMS > 0 {
		response = append(response, "match_delta_ms="+strconv.FormatInt(deltaMS, 10))
	}
	if ep.Show != "" {
		response = append(response, "show="+ep.Show)
	}
	if ep.Season > 0 && ep.Episode > 0 {
		response = append(response, fmt.Sprintf("episode=S%02dE%02d", ep.Season, ep.Episode))
	}
	if strings.TrimSpace(intent.Query) != "" {
		response = append(response, "query="+intent.Query)
	}

	for _, line := range before {
		response = append(response, "  - ["+safeTimestamp(line.Start)+"] "+line.Text)
	}
	response = append(response, "  > ["+safeTimestamp(center.Start)+"] "+center.Text)
	for _, line := range after {
		response = append(response, "  + ["+safeTimestamp(line.Start)+"] "+line.Text)
	}
	return strings.Join(response, "\n"), nil
}

func resolveCurrentSubtitle(videoPath string) (string, media.Episode) {
	cleanVideo := filepath.Clean(videoPath)
	roots := []string{
		filepath.Dir(cleanVideo),
		filepath.Dir(filepath.Dir(cleanVideo)),
	}

	empty := media.Episode{}
	for _, root := range dedupePaths(roots) {
		episodes, err := media.DiscoverEpisodes(root, 0)
		if err != nil {
			continue
		}
		for _, ep := range episodes {
			if filepath.Clean(ep.VideoPath) != cleanVideo {
				continue
			}
			if ep.SubtitlePath != "" {
				return ep.SubtitlePath, ep
			}
			return "", ep
		}
	}

	show, showSlug, season, episode := media.InferEpisodeInfo(cleanVideo)
	empty.Show = show
	empty.ShowSlug = showSlug
	empty.Season = season
	empty.Episode = episode

	subtitle, err := media.FindBestSubtitleForVideo(cleanVideo)
	if err != nil {
		return "", empty
	}
	return subtitle, empty
}

func selectSubtitleAnchorLine(lines []media.SubtitleLine, positionMS int64, query string) (int, string, int64) {
	baseIdx, baseMode, baseDelta := subtitleIndexAtPosition(lines, positionMS)
	cleanQuery := strings.ToLower(strings.TrimSpace(query))
	if cleanQuery == "" {
		return baseIdx, baseMode, baseDelta
	}

	tokens := strings.Fields(normalizeForMatch(cleanQuery))
	if len(tokens) == 0 {
		return baseIdx, baseMode, baseDelta
	}

	bestIdx := -1
	bestDelta := int64(^uint64(0) >> 1)
	for idx, line := range lines {
		lineText := normalizeForMatch(line.Text)
		match := true
		for _, token := range tokens {
			if !strings.Contains(lineText, token) {
				match = false
				break
			}
		}
		if !match {
			continue
		}

		startMS := subtitleStartMS(line)
		if startMS < 0 {
			continue
		}
		delta := absInt64(positionMS - startMS)
		if bestIdx < 0 || delta < bestDelta {
			bestIdx = idx
			bestDelta = delta
		}
	}
	if bestIdx >= 0 {
		return bestIdx, "query_near_timestamp", bestDelta
	}

	return baseIdx, baseMode + "_query_fallback", baseDelta
}

func subtitleIndexAtPosition(lines []media.SubtitleLine, positionMS int64) (int, string, int64) {
	bestPrevious := -1
	bestPreviousDelta := int64(^uint64(0) >> 1)
	bestNext := -1
	bestNextDelta := int64(^uint64(0) >> 1)

	for idx, line := range lines {
		startMS, ok := media.TimestampToMillis(line.Start)
		if !ok {
			continue
		}
		endMS, endOK := media.TimestampToMillis(line.End)
		if endOK && positionMS >= startMS && positionMS <= endMS {
			return idx, "exact_timestamp", 0
		}

		if positionMS >= startMS {
			delta := positionMS - startMS
			if bestPrevious < 0 || delta < bestPreviousDelta {
				bestPrevious = idx
				bestPreviousDelta = delta
			}
			continue
		}

		delta := startMS - positionMS
		if bestNext < 0 || delta < bestNextDelta {
			bestNext = idx
			bestNextDelta = delta
		}
	}

	if bestPrevious >= 0 {
		return bestPrevious, "closest_previous", bestPreviousDelta
	}
	if bestNext >= 0 {
		return bestNext, "closest_next", bestNextDelta
	}
	if len(lines) > 0 {
		return 0, "fallback_first_line", 0
	}
	return -1, "no_lines", 0
}

func subtitleStartMS(line media.SubtitleLine) int64 {
	startMS, ok := media.TimestampToMillis(line.Start)
	if !ok {
		return -1
	}
	return startMS
}

func subtitleContextWindow(lines []media.SubtitleLine, centerIdx, beforeCount, afterCount int) ([]media.SubtitleLine, []media.SubtitleLine) {
	if centerIdx < 0 || centerIdx >= len(lines) {
		return nil, nil
	}
	if beforeCount < 0 {
		beforeCount = 0
	}
	if afterCount < 0 {
		afterCount = 0
	}

	start := centerIdx - beforeCount
	if start < 0 {
		start = 0
	}
	end := centerIdx + afterCount + 1
	if end > len(lines) {
		end = len(lines)
	}

	before := append([]media.SubtitleLine{}, lines[start:centerIdx]...)
	after := append([]media.SubtitleLine{}, lines[centerIdx+1:end]...)
	return before, after
}

func extractAboutQuery(input string) string {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	marker := " about "
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return ""
	}
	query := strings.TrimSpace(clean[idx+len(marker):])
	query = strings.Trim(query, " \t\r\n\"'`.,!?;:()[]{}")
	if len([]rune(query)) < 3 {
		return ""
	}
	return query
}

func parsePlayerPosition(raw string) float64 {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return seconds
}

func formatPlaybackSeconds(seconds float64) string {
	total := int64(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func discoverNeighborVideos(currentPath string) ([]string, error) {
	currentDir := filepath.Dir(currentPath)
	parentDir := filepath.Dir(currentDir)

	roots := []string{currentDir}
	if parentDir != "" && parentDir != currentDir {
		entries, err := os.ReadDir(parentDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				roots = append(roots, filepath.Join(parentDir, entry.Name()))
			}
		}
	}

	seenFiles := map[string]struct{}{}
	out := make([]string, 0, 256)
	for _, root := range dedupePaths(roots) {
		if len(out) >= mediaScanFileLimit {
			break
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			depth := strings.Count(filepath.ToSlash(rel), "/")
			if d.IsDir() {
				if depth > mediaScanMaxDepth {
					return filepath.SkipDir
				}
				return nil
			}
			if depth > mediaScanMaxDepth {
				return nil
			}
			if !isVideoFile(path) {
				return nil
			}

			clean := filepath.Clean(path)
			if _, exists := seenFiles[clean]; exists {
				return nil
			}
			seenFiles[clean] = struct{}{}
			out = append(out, clean)
			if len(out) >= mediaScanFileLimit {
				return errors.New("scan limit reached")
			}
			return nil
		})
	}

	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i], out[j]) })
	return out, nil
}

func filterCandidatesByShowHint(paths []string, showHint string) []string {
	cleanHint := normalizeForMatch(showHint)
	if cleanHint == "" {
		return paths
	}
	tokens := strings.Fields(cleanHint)
	if len(tokens) == 0 {
		return paths
	}

	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := normalizeForMatch(path)
		match := true
		for _, token := range tokens {
			if !strings.Contains(normalized, token) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func pickNextPath(paths []string, currentPath string) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("no candidate files to choose from")
	}

	cleanCurrent := filepath.Clean(currentPath)
	index := -1
	for i, path := range paths {
		if filepath.Clean(path) == cleanCurrent {
			index = i
			break
		}
	}

	if index >= 0 {
		if index+1 < len(paths) {
			return paths[index+1], nil
		}
		return "", errors.New("already at the last discovered episode")
	}

	for _, path := range paths {
		if naturalLess(cleanCurrent, path) {
			return path, nil
		}
	}

	return "", errors.New("no later episode was found relative to the current file")
}

func openMediaWithVLC(player, targetPath string) (string, error) {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		absTarget = targetPath
	}
	uri := fileURI(absTarget)

	if _, err := exec.LookPath("playerctl"); err == nil && strings.TrimSpace(player) != "" {
		if _, err := playerctlValue(player, "open", uri); err == nil {
			_, _ = playerctlValue(player, "play")
			return "playerctl-open", nil
		}
	}

	if _, err := exec.LookPath("vlc"); err != nil {
		return "", errors.New("`vlc` command not found and playerctl open failed")
	}
	cmd := tracedExecCommand("vlc", "--one-instance", absTarget)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to launch vlc: %w", err)
	}
	_ = cmd.Process.Release()
	return "vlc-launch", nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func isProcessRunning(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, "pgrep", "-x", name)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func dedupePaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := videoExtensions[ext]
	return ok
}

func normalizeForMatch(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func naturalLess(a, b string) bool {
	return naturalCompare(filepath.ToSlash(strings.ToLower(a)), filepath.ToSlash(strings.ToLower(b))) < 0
}

func naturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ra := a[i]
		rb := b[j]
		if isDigitByte(ra) && isDigitByte(rb) {
			ai := i
			for i < len(a) && isDigitByte(a[i]) {
				i++
			}
			bj := j
			for j < len(b) && isDigitByte(b[j]) {
				j++
			}

			anum := strings.TrimLeft(a[ai:i], "0")
			bnum := strings.TrimLeft(b[bj:j], "0")
			if anum == "" {
				anum = "0"
			}
			if bnum == "" {
				bnum = "0"
			}
			if len(anum) != len(bnum) {
				if len(anum) < len(bnum) {
					return -1
				}
				return 1
			}
			if anum != bnum {
				if anum < bnum {
					return -1
				}
				return 1
			}
			if (i - ai) != (j - bj) {
				if (i - ai) < (j - bj) {
					return -1
				}
				return 1
			}
			continue
		}

		if ra != rb {
			if ra < rb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	if len(a) == len(b) {
		return 0
	}
	if len(a) < len(b) {
		return -1
	}
	return 1
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func safeValue(value, fallback string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return fallback
	}
	return clean
}
