package media

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	subtitleSRTTimeRE = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}[,\.]\d{3}\s+-->\s+\d{2}:\d{2}:\d{2}[,\.]\d{3}`)
	subtitleVTTTimeRE = regexp.MustCompile(`^(\d+:)?\d{2}:\d{2}\.\d{3}\s+-->\s+(\d+:)?\d{2}:\d{2}\.\d{3}`)
	subtitleNumberRE  = regexp.MustCompile(`^\d+$`)
	subtitleStyleRE   = regexp.MustCompile(`(?i)\{\\.*?\}|<[^>]+>`)
	spaceRE           = regexp.MustCompile(`\s+`)
	episodeTagSxxEyy  = regexp.MustCompile(`(?i)(.*?)\b[s](\d{1,2})[ ._-]*[e](\d{1,2})\b`)
	episodeTagX       = regexp.MustCompile(`(?i)(.*?)(\d{1,2})x(\d{1,2})\b`)
	nonAlphaNumRE     = regexp.MustCompile(`[^a-z0-9]+`)
)

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

var subtitleExtensions = map[string]struct{}{
	".srt": {},
	".vtt": {},
}

type SubtitleLine struct {
	LineNumber int
	Start      string
	End        string
	Text       string
}

type SubtitleChunk struct {
	FromLine int
	ToLine   int
	Start    string
	End      string
	Text     string
}

type SubtitleMatch struct {
	SubtitlePath string
	Show         string
	ShowSlug     string
	Season       int
	Episode      int
	Line         SubtitleLine
	Before       []SubtitleLine
	After        []SubtitleLine
}

type Episode struct {
	VideoPath    string
	SubtitlePath string
	Show         string
	ShowSlug     string
	Season       int
	Episode      int
}

func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := videoExtensions[ext]
	return ok
}

func IsSubtitleFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := subtitleExtensions[ext]
	return ok
}

func FindBestSubtitleForVideo(videoPath string) (string, error) {
	cleanVideo := strings.TrimSpace(videoPath)
	if cleanVideo == "" {
		return "", errors.New("video path is required")
	}
	absVideo, err := filepath.Abs(cleanVideo)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absVideo)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("expected file path, got directory: %s", absVideo)
	}

	dir := filepath.Dir(absVideo)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	subtitles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if IsSubtitleFile(path) {
			subtitles = append(subtitles, path)
		}
	}
	if len(subtitles) == 0 {
		return "", nil
	}
	sort.Strings(subtitles)
	return bestSubtitleForVideo(absVideo, subtitles), nil
}

func InferEpisodeInfo(path string) (show, showSlug string, season, episode int) {
	return inferEpisodeInfo(path)
}

func TimestampToMillis(value string) (int64, bool) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return 0, false
	}
	raw = strings.ReplaceAll(raw, ",", ".")

	parts := strings.Split(raw, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}

	secondsPart := parts[len(parts)-1]
	secFrac := strings.SplitN(secondsPart, ".", 2)
	seconds, err := strconv.Atoi(secFrac[0])
	if err != nil {
		return 0, false
	}

	millis := 0
	if len(secFrac) == 2 {
		fraction := secFrac[1]
		if len(fraction) > 3 {
			fraction = fraction[:3]
		}
		for len(fraction) < 3 {
			fraction += "0"
		}
		millis, err = strconv.Atoi(fraction)
		if err != nil {
			return 0, false
		}
	}

	minutes, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, false
	}
	hours := 0
	if len(parts) == 3 {
		hours, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, false
		}
	}

	total := int64(hours)*3600*1000 + int64(minutes)*60*1000 + int64(seconds)*1000 + int64(millis)
	return total, true
}

func ParseSubtitleLines(path string) ([]SubtitleLine, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".srt":
		return parseSRTLines(path)
	case ".vtt":
		return parseVTTLines(path)
	default:
		return nil, fmt.Errorf("unsupported subtitle format %q", ext)
	}
}

func ChunkSubtitleLines(lines []SubtitleLine, maxLines int) []SubtitleChunk {
	if len(lines) == 0 {
		return nil
	}
	if maxLines <= 0 {
		maxLines = 40
	}

	chunks := make([]SubtitleChunk, 0, (len(lines)/maxLines)+1)
	for start := 0; start < len(lines); start += maxLines {
		end := start + maxLines
		if end > len(lines) {
			end = len(lines)
		}
		window := lines[start:end]
		if len(window) == 0 {
			continue
		}

		textLines := make([]string, 0, len(window))
		for _, line := range window {
			label := strings.TrimSpace(line.Start)
			if label == "" {
				label = "unknown"
			}
			textLines = append(textLines, "["+label+"] "+line.Text)
		}

		chunks = append(chunks, SubtitleChunk{
			FromLine: window[0].LineNumber,
			ToLine:   window[len(window)-1].LineNumber,
			Start:    window[0].Start,
			End:      window[len(window)-1].End,
			Text:     strings.TrimSpace(strings.Join(textLines, "\n")),
		})
	}

	return chunks
}

func DiscoverEpisodes(root string, limit int) ([]Episode, error) {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		return nil, errors.New("root path is required")
	}
	absRoot, err := filepath.Abs(cleanRoot)
	if err != nil {
		return nil, err
	}

	if info, err := os.Stat(absRoot); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("media root is not a directory: %s", absRoot)
	}

	type dirState struct {
		videos    []string
		subtitles []string
	}
	byDir := map[string]*dirState{}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		dir := filepath.Dir(path)
		state := byDir[dir]
		if state == nil {
			state = &dirState{}
			byDir[dir] = state
		}

		switch {
		case IsVideoFile(path):
			state.videos = append(state.videos, path)
		case IsSubtitleFile(path):
			state.subtitles = append(state.subtitles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	out := make([]Episode, 0, 128)
	for _, dir := range dirs {
		state := byDir[dir]
		if state == nil || len(state.videos) == 0 {
			continue
		}
		sort.Strings(state.videos)
		sort.Strings(state.subtitles)

		for _, video := range state.videos {
			subtitle := bestSubtitleForVideo(video, state.subtitles)
			show, showSlug, season, episode := inferEpisodeInfo(video)
			out = append(out, Episode{
				VideoPath:    video,
				SubtitlePath: subtitle,
				Show:         show,
				ShowSlug:     showSlug,
				Season:       season,
				Episode:      episode,
			})

			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
	}

	return out, nil
}

func EpisodeTags(ep Episode) []string {
	out := []string{"media", "subtitle", "episode"}
	if ep.ShowSlug != "" {
		out = append(out, "show_"+ep.ShowSlug)
	}
	if ep.Season > 0 {
		out = append(out, fmt.Sprintf("season_%02d", ep.Season))
	}
	if ep.Episode > 0 {
		out = append(out, fmt.Sprintf("episode_%02d", ep.Episode))
	}
	if ep.Season > 0 && ep.Episode > 0 {
		out = append(out, fmt.Sprintf("s%02de%02d", ep.Season, ep.Episode))
	}
	return dedupe(out)
}

func SearchSubtitleLines(root, query string, contextWindow, limit int) ([]SubtitleMatch, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, errors.New("query is required")
	}
	if contextWindow < 0 {
		contextWindow = 0
	}
	if limit <= 0 {
		limit = 20
	}

	episodes, err := DiscoverEpisodes(root, 0)
	if err != nil {
		return nil, err
	}
	if len(episodes) == 0 {
		return nil, nil
	}

	matches := make([]SubtitleMatch, 0, limit)
	for _, ep := range episodes {
		if ep.SubtitlePath == "" {
			continue
		}
		lines, err := ParseSubtitleLines(ep.SubtitlePath)
		if err != nil {
			continue
		}
		for idx, line := range lines {
			if !strings.Contains(strings.ToLower(line.Text), query) {
				continue
			}

			start := idx - contextWindow
			if start < 0 {
				start = 0
			}
			end := idx + contextWindow + 1
			if end > len(lines) {
				end = len(lines)
			}

			before := append([]SubtitleLine{}, lines[start:idx]...)
			after := append([]SubtitleLine{}, lines[idx+1:end]...)
			matches = append(matches, SubtitleMatch{
				SubtitlePath: ep.SubtitlePath,
				Show:         ep.Show,
				ShowSlug:     ep.ShowSlug,
				Season:       ep.Season,
				Episode:      ep.Episode,
				Line:         line,
				Before:       before,
				After:        after,
			})
			if len(matches) >= limit {
				return matches, nil
			}
		}
	}

	return matches, nil
}

func bestSubtitleForVideo(video string, subtitles []string) string {
	if len(subtitles) == 0 {
		return ""
	}

	videoBase := strings.ToLower(strings.TrimSuffix(filepath.Base(video), filepath.Ext(video)))
	videoCompact := compact(videoBase)
	_, _, vs, ve := inferEpisodeInfo(video)

	type scored struct {
		path  string
		score int
	}
	best := scored{}
	bestSet := false

	for _, subtitle := range subtitles {
		subBase := strings.ToLower(strings.TrimSuffix(filepath.Base(subtitle), filepath.Ext(subtitle)))
		subCompact := compact(subBase)
		score := 0

		switch {
		case subBase == videoBase:
			score += 100
		case strings.HasPrefix(subBase, videoBase+"."):
			score += 90
		case strings.HasPrefix(subCompact, videoCompact):
			score += 80
		case strings.Contains(subCompact, videoCompact):
			score += 70
		}

		_, _, ss, se := inferEpisodeInfo(subtitle)
		if vs > 0 && ve > 0 && ss == vs && se == ve {
			score += 30
		}

		if !bestSet || score > best.score {
			best = scored{path: subtitle, score: score}
			bestSet = true
		}
	}

	if !bestSet || best.score < 60 {
		return ""
	}
	return best.path
}

func inferEpisodeInfo(path string) (show, showSlug string, season, episode int) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dirName := filepath.Base(filepath.Dir(path))
	show = cleanupShow(base)

	if m := episodeTagSxxEyy.FindStringSubmatch(base); len(m) == 4 {
		show = cleanupShow(m[1])
		season = parseInt(m[2])
		episode = parseInt(m[3])
	} else if m := episodeTagX.FindStringSubmatch(base); len(m) == 4 {
		show = cleanupShow(m[1])
		season = parseInt(m[2])
		episode = parseInt(m[3])
	}

	if strings.TrimSpace(show) == "" {
		show = cleanupShow(dirName)
	}
	showSlug = compact(show)
	return show, showSlug, season, episode
}

func parseSRTLines(path string) ([]SubtitleLine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	blocks := strings.Split(raw, "\n\n")

	out := make([]SubtitleLine, 0, len(blocks))
	lineNum := 1
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}

		idx := 0
		if subtitleNumberRE.MatchString(strings.TrimSpace(lines[idx])) && len(lines) > 1 {
			idx++
		}
		if idx >= len(lines) {
			continue
		}

		timing := strings.TrimSpace(lines[idx])
		if !subtitleSRTTimeRE.MatchString(timing) {
			continue
		}
		start, end := splitTiming(timing)
		idx++

		payload := make([]string, 0, len(lines)-idx)
		for ; idx < len(lines); idx++ {
			clean := cleanSubtitleText(lines[idx])
			if clean == "" {
				continue
			}
			payload = append(payload, clean)
		}
		if len(payload) == 0 {
			continue
		}

		out = append(out, SubtitleLine{
			LineNumber: lineNum,
			Start:      start,
			End:        end,
			Text:       strings.Join(payload, " "),
		})
		lineNum++
	}
	return out, nil
}

func parseVTTLines(path string) ([]SubtitleLine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n")
	out := make([]SubtitleLine, 0, len(lines))
	lineNum := 1

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if lower == "webvtt" || strings.HasPrefix(lower, "kind:") || strings.HasPrefix(lower, "language:") {
			continue
		}
		if strings.HasPrefix(lower, "note") {
			for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				i++
			}
			continue
		}

		timingLine := line
		if !subtitleVTTTimeRE.MatchString(timingLine) {
			if i+1 < len(lines) && subtitleVTTTimeRE.MatchString(strings.TrimSpace(lines[i+1])) {
				i++
				timingLine = strings.TrimSpace(lines[i])
			} else {
				continue
			}
		}
		start, end := splitTiming(timingLine)

		payload := make([]string, 0, 4)
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next == "" {
				break
			}
			if subtitleVTTTimeRE.MatchString(next) {
				break
			}
			i++
			clean := cleanSubtitleText(next)
			if clean == "" {
				continue
			}
			payload = append(payload, clean)
		}
		if len(payload) == 0 {
			continue
		}

		out = append(out, SubtitleLine{
			LineNumber: lineNum,
			Start:      start,
			End:        end,
			Text:       strings.Join(payload, " "),
		})
		lineNum++
	}
	return out, nil
}

func splitTiming(value string) (string, string) {
	parts := strings.Split(value, "-->")
	if len(parts) != 2 {
		return "", ""
	}

	start := strings.TrimSpace(parts[0])
	endPart := strings.TrimSpace(parts[1])
	endFields := strings.Fields(endPart)
	end := ""
	if len(endFields) > 0 {
		end = endFields[0]
	}
	return start, end
}

func cleanSubtitleText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = subtitleStyleRE.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	value = spaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func cleanupShow(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, ".", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = spaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func compact(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonAlphaNumRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func parseInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	num, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return num
}

func dedupe(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
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
