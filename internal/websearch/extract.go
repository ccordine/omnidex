package websearch

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	htmlCommentRE     = regexp.MustCompile(`(?is)<!--.*?-->`)
	headRE            = regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	scriptRE          = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRE           = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRE        = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	tagRE             = regexp.MustCompile(`(?is)<[^>]+>`)
	whitespaceRE      = regexp.MustCompile(`\s+`)
	titleRE           = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaDescriptionRE = regexp.MustCompile(`(?is)<meta[^>]+(?:name=["']description["']|property=["']og:description["'])[^>]+content=["']([^"']+)["'][^>]*>`)
)

func extractDocument(body string) (title, description, content string) {
	if match := titleRE.FindStringSubmatch(body); len(match) == 2 {
		title = truncateUTF8(normalizeHTMLText(match[1]), 240)
	}
	if match := metaDescriptionRE.FindStringSubmatch(body); len(match) == 2 {
		description = truncateUTF8(normalizeHTMLText(match[1]), 300)
	}
	return title, description, normalizeHTMLText(headRE.ReplaceAllString(body, " "))
}

func normalizeHTMLText(value string) string {
	value = htmlCommentRE.ReplaceAllString(value, " ")
	value = scriptRE.ReplaceAllString(value, " ")
	value = styleRE.ReplaceAllString(value, " ")
	value = noscriptRE.ReplaceAllString(value, " ")
	value = tagRE.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = whitespaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func truncateUTF8(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit < 1 || len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}
