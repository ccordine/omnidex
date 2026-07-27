package worker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/websearch"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Service) persistWebSearchResults(ctx context.Context, job model.Job, query string, results []websearch.Result, contexts map[string]string) (int, error) {
	if s == nil || s.repo == nil || len(results) == 0 {
		return 0, nil
	}
	baseTags := memoryScopeTags(job, parseTagsCSV(contexts["tags"]))
	tags := appendUnique(baseTags, "reference", "web_search", "research")
	if normalized := websearch.NormalizeQuery(query); normalized != "" {
		tags = appendUnique(tags, "query:"+normalized)
	}
	persisted := 0
	for i, result := range results {
		content := formatWebSearchReferenceMemory(query, result)
		if strings.TrimSpace(content) == "" {
			continue
		}
		resultTags := appendUnique(tags, "provider:"+strings.ToLower(strings.TrimSpace(result.Provider)))
		source := webSearchMemorySource(job.ID, i, result)
		var embed []float64
		if s.llm != nil {
			if vector, err := s.llm.Embedding(ctx, content); err == nil {
				embed = vector
			}
		}
		if _, err := s.repo.AddMemoryChunk(ctx, source, model.MemoryKindReference, content, resultTags, embed); err != nil {
			return persisted, err
		}
		persisted++
	}
	return persisted, nil
}

func formatWebSearchReferenceMemory(query string, result websearch.Result) string {
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return ""
	}
	lines := []string{
		"Web research reference",
		"Query: " + strings.TrimSpace(query),
		"Provider: " + strings.TrimSpace(result.Provider),
		"Title: " + strings.TrimSpace(result.Title),
		"URL: " + strings.TrimSpace(result.URL),
		"Search URL: " + strings.TrimSpace(result.SearchURL),
		"Retrieved at: " + result.RetrievedAt.UTC().Format(time.RFC3339),
		"Snippet: " + strings.TrimSpace(result.Snippet),
		"Content:",
		content,
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func webSearchMemorySource(jobID int64, index int, result websearch.Result) string {
	key := strings.Join([]string{
		strconv.FormatInt(jobID, 10),
		strings.TrimSpace(result.Provider),
		strings.TrimSpace(result.URL),
		strings.TrimSpace(result.SearchURL),
	}, "\x00")
	sum := sha1.Sum([]byte(key))
	return fmt.Sprintf("job:%d:web_search:%02d:%s", jobID, index+1, hex.EncodeToString(sum[:6]))
}

func (s *Service) deriveSearchQuery(ctx context.Context, stepID int64, job model.Job, contexts map[string]string) string {
	tags := trimForBudget(contexts["tags"], 400)
	plan := trimForBudget(contexts["plan"], 900)
	retrieval := trimForBudget(contexts["retrieval"], 900)
	feedback := trimForBudget(contexts["user_feedback"], 700)
	timeContext := currentTimeContextFromMetadata(job)

	prompt := strings.Join([]string{
		antiRoleplayInstructionForPipeline(job.Pipeline),
		"Generate one concise web-search query for the instruction.",
		"Return only the query text with no commentary.",
		"Keep it under 12 words.",
		"If the request is time-sensitive (latest/current/today/as-of), anchor the query to CURRENT_TIME_CONTEXT date.",
		"Instruction:",
		strings.TrimSpace(job.Instruction),
		"Current Time Context:",
		timeContext,
		"User Feedback:",
		feedback,
		"Plan:",
		plan,
		"Retrieved Memory:",
		retrieval,
		"Tags:",
		strings.TrimSpace(tags),
	}, "\n\n")

	searchFallback := s.specialistModel(job, specialist.RoleWebResearchSpecialist, s.models.Search)
	searchModel := metadataModel(job, "model_search", searchFallback)
	query, err := s.llmGenerateWithTrace(ctx, stepID, "search_query_derivation", searchModel, prompt)
	if err != nil {
		return ""
	}

	query = strings.TrimSpace(query)
	query = strings.Trim(query, "\"'`")
	if idx := strings.Index(query, "\n"); idx >= 0 {
		query = strings.TrimSpace(query[:idx])
	}
	query = sanitizeSearchQueryArtifacts(query)
	if len(query) > 160 {
		query = query[:160]
	}
	query = anchorTimeSensitiveQuery(query, job)
	query = sanitizeSearchQueryArtifacts(query)

	return strings.TrimSpace(query)
}

func shouldRunWebSearch(instruction string) bool {
	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	if isLocalClockOnlyInstruction(value) {
		return false
	}
	if isTimeSensitiveInstruction(value) {
		return true
	}

	if strings.Contains(value, "how do i") || strings.Contains(value, "how would i") || strings.Contains(value, "how can i") {
		return true
	}
	if strings.Contains(value, "research") || strings.Contains(value, "look up") {
		return true
	}
	if shouldForceFreshWebSearch(value, "") {
		return true
	}

	return webSearchKeywordPattern.MatchString(value)
}

func isTimeSensitiveInstruction(instruction string) bool {
	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	if relativeTimePattern.MatchString(value) {
		return true
	}
	phrases := []string{
		"as of",
		"up to date",
		"at the moment",
		"happening now",
		"current status",
		"latest status",
	}
	for _, phrase := range phrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func isLocalClockOnlyInstruction(instruction string) bool {
	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	if !localClockQuestionPattern.MatchString(value) {
		return false
	}
	nonLocalSignals := []string{
		"stock",
		"price",
		"weather",
		"news",
		"market",
		"exchange",
		"release",
		"sports",
		"election",
	}
	for _, signal := range nonLocalSignals {
		if strings.Contains(value, signal) {
			return false
		}
	}
	return true
}

func shouldForceFreshWebSearch(instruction, feedback string) bool {
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		instruction,
		feedback,
	}, "\n")))
	if text == "" {
		return false
	}
	if explicitWebRequestPattern.MatchString(text) {
		return true
	}
	return staleMemoryPattern.MatchString(text)
}

func shouldBypassHistoricalContext(instruction, feedback string) bool {
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		instruction,
		feedback,
	}, "\n")))
	if text == "" {
		return false
	}
	if staleMemoryPattern.MatchString(text) {
		return true
	}
	return explicitFreshContextPattern.MatchString(text)
}

func isFollowUpStatusCheckInstruction(instruction string, pipeline string) bool {
	if strings.ToLower(strings.TrimSpace(pipeline)) != model.PipelineChat {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	value = strings.Trim(value, "\"'`.,!?;:()[]{}")
	if value == "" {
		return false
	}

	patterns := []string{
		"is it done",
		"is that done",
		"done?",
		"is this done",
		"did you do it",
		"did that finish",
		"did it finish",
		"did it work",
		"is it finished",
	}
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func parentJobID(job model.Job) int64 {
	value, ok := metadataValue(job.Metadata, "parent_job_id")
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return int64(typed)
		}
	case int:
		if typed > 0 {
			return int64(typed)
		}
	case int64:
		if typed > 0 {
			return typed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func clientCWDForJob(job model.Job) string {
	return strings.TrimSpace(metadataString(job.Metadata, "client_cwd"))
}

func simpleFileTaskTarget(job model.Job) string {
	if requested := parseRequestedFileTarget(job.Instruction); requested != "" {
		return filepath.Clean(requested)
	}
	if inferred := inferNamedTypedFileTarget(job.Instruction); inferred != "" {
		return inferred
	}
	return "test"
}

func testFilePathForJob(job model.Job) string {
	target := simpleFileTaskTarget(job)
	cwd := clientCWDForJob(job)
	if cwd == "" {
		return target
	}
	return filepath.Join(cwd, target)
}

func verifyTestFileCommand(job model.Job) string {
	targetPath := testFilePathForJob(job)
	if targetPath == "test" {
		return "ls -l test"
	}
	return fmt.Sprintf("ls -l %q", targetPath)
}

func inferNamedTypedFileTarget(request string) string {
	request = strings.TrimSpace(request)
	if request == "" {
		return ""
	}
	matches := namedTypedFilePattern.FindStringSubmatch(request)
	if len(matches) != 3 {
		return ""
	}
	name := sanitizeFileTargetToken(matches[1])
	ext := normalizeNamedFileTypeExtension(matches[2])
	if name == "" || ext == "" {
		return ""
	}
	if strings.Contains(filepath.Base(name), ".") {
		return filepath.Clean(name)
	}
	return filepath.Clean(name + "." + ext)
}

func normalizeNamedFileTypeExtension(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "html":
		return "html"
	case "css":
		return "css"
	case "js", "javascript":
		return "js"
	case "json":
		return "json"
	case "md", "markdown":
		return "md"
	case "txt", "text":
		return "txt"
	default:
		return ""
	}
}

func (s *Service) parentJobSummary(ctx context.Context, job model.Job) string {
	parentID := parentJobID(job)
	if parentID <= 0 {
		return ""
	}
	parent, err := s.repo.GetJobDetails(ctx, parentID)
	if err != nil {
		return fmt.Sprintf("parent_job_id=%d parent_job_status=unknown", parentID)
	}
	result := safeLine(trimForBudget(parent.Job.Result, 180), "(empty)")
	return strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("parent_job_id=%d", parent.Job.ID),
		"parent_job_status=" + safeLine(parent.Job.Status, "unknown"),
		"parent_job_result=" + result,
	}, " "))
}

func (s *Service) followUpStatusResponse(ctx context.Context, job model.Job) string {
	verifyCmd := verifyTestFileCommand(job)
	parentID := parentJobID(job)
	if parentID <= 0 {
		return "I can’t confirm completion from this turn alone. I only report actions in chat unless a command was actually run."
	}

	parent, err := s.repo.GetJobDetails(ctx, parentID)
	if err != nil {
		return fmt.Sprintf("I couldn’t load the previous turn state. Please run `%s` in your shell to verify.", verifyCmd)
	}

	parentResult := strings.ToLower(strings.TrimSpace(parent.Job.Result))
	switch {
	case strings.Contains(parentResult, "run `touch test`") || strings.Contains(parentResult, "touch test"):
		return fmt.Sprintf("Not yet. I only suggested the command `touch test`; I did not execute it in your shell. Verify with `%s`.", verifyCmd)
	case parent.Job.Status == model.JobStatusCompleted:
		return fmt.Sprintf("The previous turn completed, but chat mode may only provide instructions. Verify with `%s`.", verifyCmd)
	case parent.Job.Status == model.JobStatusRunning || parent.Job.Status == model.JobStatusPending:
		return fmt.Sprintf("Not yet. The previous turn is still %s.", parent.Job.Status)
	default:
		return fmt.Sprintf("Not yet. The previous turn status is %s.", parent.Job.Status)
	}
}

func shouldAttachRecentConversation(job model.Job, action string) bool {
	if !strings.EqualFold(strings.TrimSpace(job.Pipeline), model.PipelineChat) {
		return false
	}
	if strings.TrimSpace(metadataString(job.Metadata, "session_id")) == "" {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(action)) {
	case "plan", "tag", "retrieve", "web_search", "analyze", "assist", "roleplay", "narrate", "verify":
		return true
	default:
		return false
	}
}

func (s *Service) recentConversationContext(ctx context.Context, job model.Job) string {
	sessionID := strings.TrimSpace(metadataString(job.Metadata, "session_id"))
	if sessionID == "" {
		return ""
	}
	turns, err := s.repo.ListRecentSessionJobs(ctx, model.PipelineChat, sessionID, job.ID, recentConversationTurnLimit)
	if err != nil {
		s.logger.Printf("job=%d recent conversation lookup failed: %v", job.ID, err)
		return ""
	}
	return formatRecentConversationTurns(turns, recentConversationContextBudget)
}

func formatRecentConversationTurns(turns []model.Job, budget int) string {
	if len(turns) == 0 {
		return ""
	}
	if budget <= 0 {
		budget = recentConversationContextBudget
	}

	segments := make([]string, 0, len(turns))
	for _, turn := range turns {
		userText := safeLine(trimForBudget(turn.Instruction, recentConversationTurnBudget), "")
		assistantText := safeLine(trimForBudget(turn.Result, recentConversationTurnBudget), "(no assistant response captured)")
		if userText == "" && strings.TrimSpace(turn.Result) == "" {
			continue
		}
		segment := strings.Join([]string{
			fmt.Sprintf("turn_id=%d status=%s", turn.ID, safeLine(turn.Status, "unknown")),
			"user: " + userText,
			"assistant: " + assistantText,
		}, "\n")
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return ""
	}

	return trimForBudget("Recent session conversation (oldest to newest):\n\n"+strings.Join(segments, "\n\n"), budget)
}

func isSimpleFileTaskInstruction(instruction string, pipeline string) bool {
	if strings.ToLower(strings.TrimSpace(pipeline)) != model.PipelineChat {
		return false
	}

	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	padded := " " + value + " "

	hasCreateIntent := strings.Contains(padded, " create ") ||
		strings.Contains(padded, " make ") ||
		strings.Contains(padded, " new ") ||
		strings.Contains(padded, " write ")
	hasFileTarget := strings.Contains(padded, " file ") ||
		strings.Contains(padded, " document ") ||
		strings.Contains(padded, " doc ")
	hasExplicitFilename := filePathTokenPattern.MatchString(value)
	if !hasCreateIntent || (!hasFileTarget && !hasExplicitFilename) {
		return false
	}

	complexTokens := []string{" docker ", " container ", " kubernetes ", " repo ", " repository ", " project ", " deploy ", " migration "}
	for _, token := range complexTokens {
		if strings.Contains(padded, token) {
			return false
		}
	}

	return true
}

func shouldForceCodeOnlyResponse(job model.Job, contexts map[string]string, modelName string) bool {
	if isDeterministicLocalActionReviewInstruction(job.Instruction) {
		return false
	}

	for _, key := range []string{"response_code_only", "code_only", "raw_code_only"} {
		if metadataBool(job.Metadata, key, false) {
			return true
		}
	}

	for _, key := range []string{"response_mode", "output_mode", "response_format", "response_style"} {
		mode := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, key)))
		switch mode {
		case "code", "code_only", "raw_code", "raw_file", "raw":
			return true
		}
	}

	preferenceText := strings.Join([]string{
		job.Instruction,
		strings.TrimSpace(contexts["user_feedback"]),
		strings.TrimSpace(contexts["replan_feedback"]),
	}, "\n")
	if codeOnlyPreferencePattern.MatchString(strings.ToLower(preferenceText)) {
		return true
	}

	if !isCodeGenerationRequest(job.Instruction, contexts) {
		return false
	}

	return isCoderModelName(modelName)
}

func isCoderModelName(modelName string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	if lower == "" {
		return false
	}
	markers := []string{
		"coder",
		"codegemma",
		"codellama",
		"starcoder",
		"wizardcoder",
		"codestral",
		"devstral",
		"deepseek-coder",
		"qwen3-coder",
		"qwen2.5-coder",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isCodeGenerationRequest(instruction string, contexts map[string]string) bool {
	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	if codeOnlyPreferencePattern.MatchString(value) {
		return true
	}

	for _, token := range filePathTokenPattern.FindAllString(value, -1) {
		if hasCodeLikeExtension(token) {
			return true
		}
	}

	actionMarkers := []string{
		"write",
		"create",
		"make",
		"generate",
		"build",
		"implement",
		"draft",
		"compose",
		"return",
		"output",
		"produce",
	}
	codeMarkers := []string{
		"code",
		"function",
		"class",
		"script",
		"snippet",
		"source",
		"html",
		"css",
		"javascript",
		"typescript",
		"python",
		"golang",
		"go ",
		"rust",
		"java",
		"sql",
		"yaml",
		"json",
		"xml",
		"dockerfile",
		"bash",
		"shell",
	}
	if containsAnyMarker(value, actionMarkers) && containsAnyMarker(value, codeMarkers) {
		return true
	}

	if strings.Contains(value, "file") && (strings.Contains(value, "content") || strings.Contains(value, "contents")) {
		return true
	}

	tags := strings.ToLower(strings.TrimSpace(contexts["tags"]))
	if tags != "" && containsAnyMarker(tags, []string{"code", "programming", "html", "javascript", "python", "go", "sql"}) && containsAnyMarker(value, actionMarkers) {
		return true
	}

	return false
}

func hasCodeLikeExtension(path string) bool {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(path)))
	switch ext {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx",
		".rs", ".java", ".kt", ".swift", ".cs", ".php", ".rb",
		".c", ".h", ".cc", ".cpp", ".hpp",
		".html", ".htm", ".css", ".scss", ".sass",
		".json", ".yaml", ".yml", ".toml", ".ini", ".xml",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".sql", ".graphql":
		return true
	default:
		return false
	}
}

func containsAnyMarker(value string, markers []string) bool {
	if strings.TrimSpace(value) == "" || len(markers) == 0 {
		return false
	}
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func normalizeCodeOnlyResponse(text string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}

	if fenced := extractCodeFromFences(clean); fenced != "" {
		clean = fenced
	}

	clean = stripSourcesSectionFromResponse(clean)
	clean = strings.ReplaceAll(clean, "```", "")
	lines := trimLikelyProseBoundaries(strings.Split(clean, "\n"))
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func extractCodeFromFences(text string) string {
	lines := strings.Split(text, "\n")
	inFence := false
	sawFence := false
	segments := make([]string, 0, 2)
	current := make([]string, 0, 24)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			sawFence = true
			if inFence {
				segment := strings.TrimSpace(strings.Join(current, "\n"))
				if segment != "" {
					segments = append(segments, segment)
				}
				current = current[:0]
				inFence = false
			} else {
				inFence = true
			}
			continue
		}
		if inFence {
			current = append(current, line)
		}
	}

	if inFence {
		segment := strings.TrimSpace(strings.Join(current, "\n"))
		if segment != "" {
			segments = append(segments, segment)
		}
	}

	if !sawFence || len(segments) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(segments, "\n\n"))
}
