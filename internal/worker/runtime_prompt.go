package worker

import "strings"

func promptBlock(name, value string) string {
	label := normalizePromptBlockName(name)
	return "<" + label + ">\n" + sanitizePromptBlockBody(value) + "\n</" + label + ">"
}

func sanitizePromptBlockBody(value string) string {
	body := strings.TrimSpace(value)
	if body == "" {
		return "(empty)"
	}
	body = strings.ReplaceAll(body, "\x00", "")
	body = strings.ReplaceAll(body, "<", "&lt;")
	return strings.ReplaceAll(body, ">", "&gt;")
}

func normalizePromptBlockName(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var output strings.Builder
	previousSeparator := false
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			output.WriteRune(character)
			previousSeparator = false
			continue
		}
		if output.Len() > 0 && !previousSeparator {
			output.WriteByte('_')
			previousSeparator = true
		}
	}
	if result := strings.Trim(output.String(), "_"); result != "" {
		return result
	}
	return "SECTION"
}
