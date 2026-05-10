package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const screenCaptureTimeout = 8 * time.Second
const screenOCRTimeout = 10 * time.Second
const screenVisionTimeout = 45 * time.Second

type screenReadIntent struct {
	WithOCR    bool
	WithVision bool
	Prompt     string
}

type screenReadResult struct {
	GeneratedAt   string   `json:"generated_at"`
	CaptureTool   string   `json:"capture_tool"`
	ImagePath     string   `json:"image_path,omitempty"`
	OCRText       string   `json:"ocr_text,omitempty"`
	VisionModel   string   `json:"vision_model,omitempty"`
	VisionSummary string   `json:"vision_summary,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type ollamaGenerateRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images,omitempty"`
	Stream bool     `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

func tryHandleLocalScreenCommand(input string) (bool, string) {
	intent, ok := parseScreenReadIntent(input)
	if !ok {
		return false, ""
	}

	result, err := screenReadReport(intent.WithOCR, intent.WithVision, intent.Prompt, defaultVisionModel(), defaultOllamaBaseURL(), false)
	if err != nil {
		return true, "Local screen action failed: " + err.Error()
	}
	return true, screenReadToText(result)
}

func parseScreenReadIntent(input string) (screenReadIntent, bool) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return screenReadIntent{}, false
	}
	lower := strings.ToLower(clean)

	screenCue := containsAnyPhrase(lower, []string{"screen", "screenshot", "display", "monitor"})
	actionCue := containsAnyPhrase(lower, []string{"read", "describe", "summarize", "what's on", "what is on", "scan", "capture", "take"})
	if !screenCue || !actionCue {
		return screenReadIntent{}, false
	}

	intent := screenReadIntent{
		WithOCR:    true,
		WithVision: false,
	}
	if containsAnyPhrase(lower, []string{"what's on", "what is on", "describe", "summarize", "look at", "ui", "layout", "button", "icon"}) {
		intent.WithVision = true
	}
	if containsAnyPhrase(lower, []string{"only text", "ocr only", "text only"}) {
		intent.WithOCR = true
		intent.WithVision = false
	}
	if containsAnyPhrase(lower, []string{"vision only", "image only"}) {
		intent.WithOCR = false
		intent.WithVision = true
	}

	intent.Prompt = extractScreenPrompt(clean)
	return intent, true
}

func extractScreenPrompt(input string) string {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	for _, marker := range []string{" about ", " for ", " focusing on "} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		prompt := strings.TrimSpace(clean[idx+len(marker):])
		prompt = strings.Trim(prompt, " \t\r\n\"'`.,!?;:()[]{}")
		return prompt
	}
	return ""
}

func runScreenRead(args []string) {
	fs := flag.NewFlagSet("screen-read", flag.ExitOnError)
	withOCR := fs.Bool("ocr", true, "extract text with OCR (tesseract)")
	withVision := fs.Bool("vision", false, "send screenshot to Ollama vision model")
	prompt := fs.String("prompt", "", "optional focus prompt for vision summary")
	model := fs.String("model", defaultVisionModel(), "vision model (for --vision), e.g. llava:latest")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL for --vision")
	keep := fs.Bool("keep", false, "keep captured screenshot file")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	_ = fs.Parse(args)

	result, err := screenReadReport(*withOCR, *withVision, *prompt, *model, *baseURL, *keep)
	if err != nil {
		die(err.Error())
	}

	if *jsonOutput {
		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
		return
	}

	fmt.Println(screenReadToText(result))
}

func screenReadReport(withOCR, withVision bool, prompt, visionModel, ollamaBaseURL string, keep bool) (screenReadResult, error) {
	if !withOCR && !withVision {
		withOCR = true
	}
	if err := ensureLocalPermission(permissionKeyScreenCapture, "Allow capturing a local screenshot from your active display."); err != nil {
		return screenReadResult{}, err
	}

	warnings := make([]string, 0, 3)
	if withOCR {
		if err := ensureLocalPermission(permissionKeyScreenOCR, "Allow OCR text extraction from captured screenshots."); err != nil {
			withOCR = false
			warnings = append(warnings, err.Error())
		}
	}
	if withVision {
		visionReason := "Allow sending captured screenshots to local Ollama vision model at " + screenGenerateEndpoint(ollamaBaseURL) + "."
		if err := ensureLocalPermission(permissionKeyScreenVision, visionReason); err != nil {
			withVision = false
			warnings = append(warnings, err.Error())
		}
	}
	if !withOCR && !withVision {
		if len(warnings) == 0 {
			warnings = append(warnings, "all screen reading operations are disabled")
		}
		return screenReadResult{}, errors.New(strings.Join(warnings, "; "))
	}

	imagePath, tool, err := captureScreenImage()
	if err != nil {
		return screenReadResult{}, err
	}
	if !keep {
		defer os.Remove(imagePath)
	}

	result := screenReadResult{
		GeneratedAt: time.Now().Format(time.RFC3339),
		CaptureTool: tool,
	}
	if keep {
		result.ImagePath = imagePath
	}
	if len(warnings) > 0 {
		result.Warnings = append(result.Warnings, warnings...)
	}
	if withOCR {
		text, err := runScreenOCR(imagePath)
		if err != nil {
			warnings = append(warnings, "ocr: "+err.Error())
		} else {
			result.OCRText = text
		}
	}

	if withVision {
		visionPrompt := strings.TrimSpace(prompt)
		if visionPrompt == "" {
			visionPrompt = "Describe what is currently visible on this screen, including key windows, text, and actionable UI elements."
		}
		summary, err := runOllamaVision(ollamaBaseURL, visionModel, visionPrompt, imagePath)
		if err != nil {
			warnings = append(warnings, "vision: "+err.Error())
		} else {
			result.VisionModel = strings.TrimSpace(visionModel)
			result.VisionSummary = summary
		}
	}

	if len(warnings) > 0 {
		result.Warnings = warnings
	}
	if strings.TrimSpace(result.OCRText) == "" && strings.TrimSpace(result.VisionSummary) == "" {
		if len(warnings) == 0 {
			warnings = append(warnings, "no OCR or vision output produced")
		}
		return result, errors.New(strings.Join(warnings, "; "))
	}

	return result, nil
}

func captureScreenImage() (string, string, error) {
	tmp, err := os.CreateTemp("", "omni-screen-*.png")
	if err != nil {
		return "", "", err
	}
	path := tmp.Name()
	_ = tmp.Close()

	type candidate struct {
		name string
		args []string
	}

	candidates := []candidate{
		{name: "grim", args: []string{"-t", "png", path}},
		{name: "gnome-screenshot", args: []string{"-f", path}},
		{name: "maim", args: []string{path}},
		{name: "scrot", args: []string{path}},
		{name: "import", args: []string{"-window", "root", path}},
	}

	attemptErrors := make([]string, 0, len(candidates))
	foundTool := false
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate.name); err != nil {
			continue
		}
		foundTool = true

		ctx, cancel := context.WithTimeout(context.Background(), screenCaptureTimeout)
		cmd := tracedExecCommandContext(ctx, candidate.name, candidate.args...)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			attemptErrors = append(attemptErrors, candidate.name+": "+truncateScreenText(text, 200))
			continue
		}

		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
			return path, candidate.name, nil
		}
		attemptErrors = append(attemptErrors, candidate.name+": empty screenshot output")
	}

	_ = os.Remove(path)
	if !foundTool {
		return "", "", errors.New("no screenshot utility found (install grim, gnome-screenshot, maim, scrot, or ImageMagick import)")
	}
	return "", "", fmt.Errorf("failed to capture screen: %s", strings.Join(attemptErrors, " | "))
}

func runScreenOCR(imagePath string) (string, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", errors.New("tesseract not found (install tesseract-ocr for screen text extraction)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), screenOCRTimeout)
	defer cancel()
	cmd := tracedExecCommandContext(ctx, "tesseract", imagePath, "stdout", "--psm", "6")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(truncateScreenText(text, 240))
	}

	normalized := normalizeScreenText(text)
	if normalized == "" {
		return "", errors.New("no readable text detected")
	}
	return normalized, nil
}

func runOllamaVision(baseURL, model, prompt, imagePath string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", errors.New("vision model is required (set --model or OLLAMA_MODEL_VISION)")
	}
	endpoint := screenGenerateEndpoint(baseURL)
	if endpoint == "" {
		return "", errors.New("invalid Ollama base URL")
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(data)

	requestBody := ollamaGenerateRequest{
		Model:  model,
		Prompt: strings.TrimSpace(prompt),
		Images: []string{encoded},
		Stream: false,
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), screenVisionTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: screenVisionTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama status=%d body=%s", resp.StatusCode, truncateScreenText(strings.TrimSpace(string(raw)), 240))
	}

	var parsed ollamaGenerateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse ollama response: %w", err)
	}
	if strings.TrimSpace(parsed.Error) != "" {
		return "", errors.New(strings.TrimSpace(parsed.Error))
	}
	result := normalizeScreenText(parsed.Response)
	if result == "" {
		return "", errors.New("empty vision response")
	}
	return result, nil
}

func screenGenerateEndpoint(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = defaultOllamaBaseURL()
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, "/api/generate") {
		return base
	}
	return base + "/api/generate"
}

func defaultVisionModel() string {
	if value := strings.TrimSpace(os.Getenv("OLLAMA_MODEL_VISION")); value != "" {
		return value
	}
	return "llava:latest"
}

func defaultOllamaBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")); value != "" {
		return value
	}
	return "http://localhost:11434"
}

func normalizeScreenText(value string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return ""
	}
	lines := strings.Split(clean, "\n")
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		trimmed = append(trimmed, line)
	}
	return strings.Join(trimmed, "\n")
}

func screenReadToText(result screenReadResult) string {
	lines := []string{
		"Local screen read:",
		"generated_at=" + safeValue(result.GeneratedAt, "unknown"),
		"capture_tool=" + safeValue(result.CaptureTool, "unknown"),
	}
	if strings.TrimSpace(result.ImagePath) != "" {
		lines = append(lines, "image_path="+result.ImagePath)
	}
	if strings.TrimSpace(result.OCRText) != "" {
		lines = append(lines, "ocr_text:")
		lines = append(lines, truncateScreenText(result.OCRText, 1800))
	}
	if strings.TrimSpace(result.VisionSummary) != "" {
		lines = append(lines, "vision_summary (model="+safeValue(result.VisionModel, "unknown")+"):")
		lines = append(lines, truncateScreenText(result.VisionSummary, 1800))
	}
	if len(result.Warnings) > 0 {
		lines = append(lines, "warnings:")
		for _, warning := range result.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	return strings.Join(lines, "\n")
}

func truncateScreenText(value string, maxRunes int) string {
	clean := strings.TrimSpace(value)
	if maxRunes <= 0 {
		return clean
	}
	runes := []rune(clean)
	if len(runes) <= maxRunes {
		return clean
	}
	return string(runes[:maxRunes]) + "..."
}
