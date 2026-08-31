package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	screenCaptureTimeout = 8 * time.Second
	screenOCRTimeout     = 10 * time.Second
)

type screenReadResult struct {
	GeneratedAt string   `json:"generated_at"`
	CaptureTool string   `json:"capture_tool"`
	ImagePath   string   `json:"image_path,omitempty"`
	OCRText     string   `json:"ocr_text,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

func runScreenRead(args []string) {
	if retired := retiredScreenReadFlag(args); retired != "" {
		die(fmt.Sprintf("screen-read flag %s was removed; screen-read supports deterministic screenshot capture and OCR only", retired))
	}
	fs := flag.NewFlagSet("screen-read", flag.ExitOnError)
	withOCR := fs.Bool("ocr", true, "extract text with OCR (tesseract)")
	keep := fs.Bool("keep", false, "keep captured screenshot file")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		die("screen-read does not accept positional arguments")
	}

	result, err := screenReadReport(*withOCR, *keep)
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

func retiredScreenReadFlag(args []string) string {
	for _, argument := range args {
		if !strings.HasPrefix(argument, "-") {
			continue
		}
		name := strings.TrimLeft(argument, "-")
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			name = name[:separator]
		}
		switch name {
		case "vision", "prompt", "model", "base-url":
			return "--" + name
		}
	}
	return ""
}

func screenReadReport(withOCR, keep bool) (screenReadResult, error) {
	if !withOCR {
		return screenReadResult{}, errors.New("screen-read requires OCR; --ocr=false is unsupported")
	}
	if err := ensureLocalPermission(permissionKeyScreenCapture, "Allow capturing a local screenshot from your active display."); err != nil {
		return screenReadResult{}, err
	}
	if err := ensureLocalPermission(permissionKeyScreenOCR, "Allow OCR text extraction from captured screenshots."); err != nil {
		return screenReadResult{}, err
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
	text, err := runScreenOCR(imagePath)
	if err != nil {
		warning := "ocr: " + err.Error()
		result.Warnings = []string{warning}
		return result, errors.New(warning)
	}
	result.OCRText = text

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
