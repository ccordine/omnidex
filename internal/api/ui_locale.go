package api

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type uiLocale string

const (
	uiLocaleEnglish           uiLocale = "en"
	uiLocaleSpanish           uiLocale = "es"
	uiLocaleChineseSimplified uiLocale = "zh-Hans"
	uiLocaleRussian           uiLocale = "ru"
	uiLocaleJapanese          uiLocale = "ja"
)

type uiLocaleOption struct {
	Code        uiLocale
	Label       string
	NativeLabel string
	Dir         string
}

var supportedUILocaleOptions = []uiLocaleOption{
	{Code: uiLocaleEnglish, Label: "English", NativeLabel: "English", Dir: "ltr"},
	{Code: uiLocaleSpanish, Label: "Spanish", NativeLabel: "Español", Dir: "ltr"},
	{Code: uiLocaleChineseSimplified, Label: "Chinese (Simplified)", NativeLabel: "简体中文", Dir: "ltr"},
	{Code: uiLocaleRussian, Label: "Russian", NativeLabel: "Русский", Dir: "ltr"},
	{Code: uiLocaleJapanese, Label: "Japanese", NativeLabel: "日本語", Dir: "ltr"},
}

func parseUILocale(value string) (uiLocale, error) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if normalized == "" {
		return "", fmt.Errorf("UI locale is required")
	}
	parts := strings.Split(normalized, "-")
	switch parts[0] {
	case "en":
		return uiLocaleEnglish, nil
	case "es":
		return uiLocaleSpanish, nil
	case "ru":
		return uiLocaleRussian, nil
	case "ja":
		return uiLocaleJapanese, nil
	case "zh":
		for _, part := range parts[1:] {
			switch part {
			case "hant", "tw", "hk", "mo":
				return "", fmt.Errorf("unsupported UI locale %q: Traditional Chinese is not configured", value)
			}
		}
		return uiLocaleChineseSimplified, nil
	default:
		return "", fmt.Errorf("unsupported UI locale %q", value)
	}
}

func negotiateUILocale(header string) uiLocale {
	type candidate struct {
		tag   string
		q     float64
		order int
	}
	candidates := make([]candidate, 0, 4)
	for order, raw := range strings.Split(header, ",") {
		parts := strings.Split(raw, ";")
		tag := strings.TrimSpace(parts[0])
		if tag == "" {
			continue
		}
		quality := 1.0
		valid := true
		for _, parameter := range parts[1:] {
			name, value, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || parsed < 0 || parsed > 1 {
				valid = false
				break
			}
			quality = parsed
		}
		if valid && quality > 0 {
			candidates = append(candidates, candidate{tag: tag, q: quality, order: order})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].q == candidates[j].q {
			return candidates[i].order < candidates[j].order
		}
		return candidates[i].q > candidates[j].q
	})
	for _, item := range candidates {
		if item.tag == "*" {
			return uiLocaleEnglish
		}
		locale, err := parseUILocale(item.tag)
		if err == nil {
			return locale
		}
	}
	return uiLocaleEnglish
}

func uiLocaleOptionFor(locale uiLocale) (uiLocaleOption, error) {
	for _, option := range supportedUILocaleOptions {
		if option.Code == locale {
			return option, nil
		}
	}
	return uiLocaleOption{}, fmt.Errorf("unsupported UI locale %q", locale)
}
