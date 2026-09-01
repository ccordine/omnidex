package api

import (
	"fmt"
	"html"
	"strings"
	"unicode"
)

type localizedAttribute struct {
	Marker string
	Target string
}

var localizedHTMLAttributes = []localizedAttribute{
	{Marker: "data-i18n-placeholder", Target: "placeholder"},
	{Marker: "data-i18n-title", Target: "title"},
	{Marker: "data-i18n-aria", Target: "aria-label"},
}

func renderLocalizedHTML(source string, locale uiLocale) (string, error) {
	option, err := uiLocaleOptionFor(locale)
	if err != nil {
		return "", err
	}
	catalog, err := loadUIMessageCatalog(locale)
	if err != nil {
		return "", err
	}
	rendered, err := localizeElementText(source, locale, catalog)
	if err != nil {
		return "", err
	}
	for _, attribute := range localizedHTMLAttributes {
		rendered, err = localizeElementAttribute(rendered, locale, catalog, attribute)
		if err != nil {
			return "", err
		}
	}
	rendered, err = renderUILocaleOptions(rendered, locale)
	if err != nil {
		return "", err
	}
	if strings.Contains(rendered, "<html") {
		rendered, err = setDocumentLocale(rendered, option)
		if err != nil {
			return "", err
		}
	}
	rendered = stripLocalizationMarkers(rendered)
	return rendered, nil
}

func localizeElementText(source string, locale uiLocale, catalog map[string]string) (string, error) {
	const marker = `data-i18n="`
	rendered := source
	cursor := 0
	for {
		relative := strings.Index(rendered[cursor:], marker)
		if relative < 0 {
			return rendered, nil
		}
		markerStart := cursor + relative
		keyStart := markerStart + len(marker)
		keyEndRelative := strings.IndexByte(rendered[keyStart:], '"')
		if keyEndRelative < 0 {
			return "", fmt.Errorf("unterminated data-i18n attribute")
		}
		key := rendered[keyStart : keyStart+keyEndRelative]
		message, err := uiMessage(catalog, locale, key)
		if err != nil {
			return "", err
		}
		tagStart := strings.LastIndex(rendered[:markerStart], "<")
		tagEndRelative := strings.IndexByte(rendered[markerStart:], '>')
		if tagStart < 0 || tagEndRelative < 0 {
			return "", fmt.Errorf("message %q is not inside a complete opening tag", key)
		}
		tagEnd := markerStart + tagEndRelative
		tagName, err := openingTagName(rendered[tagStart : tagEnd+1])
		if err != nil {
			return "", fmt.Errorf("message %q: %w", key, err)
		}
		contentStart := tagEnd + 1
		closingTag := "</" + tagName + ">"
		closingRelative := strings.Index(rendered[contentStart:], closingTag)
		if closingRelative < 0 {
			return "", fmt.Errorf("message %q element <%s> has no closing tag", key, tagName)
		}
		contentEnd := contentStart + closingRelative
		if strings.Contains(rendered[contentStart:contentEnd], "<") {
			return "", fmt.Errorf("message %q element <%s> contains nested markup", key, tagName)
		}
		escaped := html.EscapeString(message)
		rendered = rendered[:contentStart] + escaped + rendered[contentEnd:]
		cursor = contentStart + len(escaped) + len(closingTag)
	}
}

func localizeElementAttribute(source string, locale uiLocale, catalog map[string]string, attribute localizedAttribute) (string, error) {
	marker := attribute.Marker + `="`
	rendered := source
	cursor := 0
	for {
		relative := strings.Index(rendered[cursor:], marker)
		if relative < 0 {
			return rendered, nil
		}
		markerStart := cursor + relative
		keyStart := markerStart + len(marker)
		keyEndRelative := strings.IndexByte(rendered[keyStart:], '"')
		if keyEndRelative < 0 {
			return "", fmt.Errorf("unterminated %s attribute", attribute.Marker)
		}
		key := rendered[keyStart : keyStart+keyEndRelative]
		message, err := uiMessage(catalog, locale, key)
		if err != nil {
			return "", err
		}
		tagStart := strings.LastIndex(rendered[:markerStart], "<")
		tagEndRelative := strings.IndexByte(rendered[markerStart:], '>')
		if tagStart < 0 || tagEndRelative < 0 {
			return "", fmt.Errorf("message %q is not inside a complete opening tag", key)
		}
		tagEnd := markerStart + tagEndRelative
		tag := rendered[tagStart : tagEnd+1]
		updated, err := setHTMLAttribute(tag, attribute.Target, message, true)
		if err != nil {
			return "", fmt.Errorf("message %q: %w", key, err)
		}
		rendered = rendered[:tagStart] + updated + rendered[tagEnd+1:]
		cursor = tagStart + len(updated)
	}
}

func renderUILocaleOptions(source string, selected uiLocale) (string, error) {
	const marker = "data-ui-locale-select"
	rendered := source
	cursor := 0
	for {
		relative := strings.Index(rendered[cursor:], marker)
		if relative < 0 {
			return rendered, nil
		}
		markerStart := cursor + relative
		tagStart := strings.LastIndex(rendered[:markerStart], "<")
		tagEndRelative := strings.IndexByte(rendered[markerStart:], '>')
		if tagStart < 0 || tagEndRelative < 0 {
			return "", fmt.Errorf("locale selector is not inside a complete opening tag")
		}
		tagEnd := markerStart + tagEndRelative
		tagName, err := openingTagName(rendered[tagStart : tagEnd+1])
		if err != nil || tagName != "select" {
			return "", fmt.Errorf("data-ui-locale-select must be placed on a select element")
		}
		contentStart := tagEnd + 1
		closingRelative := strings.Index(rendered[contentStart:], "</select>")
		if closingRelative < 0 {
			return "", fmt.Errorf("locale selector has no closing tag")
		}
		contentEnd := contentStart + closingRelative
		var options strings.Builder
		for _, option := range supportedUILocaleOptions {
			options.WriteString(`<option value="`)
			options.WriteString(html.EscapeString(string(option.Code)))
			options.WriteByte('"')
			if option.Code == selected {
				options.WriteString(" selected")
			}
			options.WriteByte('>')
			options.WriteString(html.EscapeString(option.NativeLabel))
			options.WriteString("</option>")
		}
		rendered = rendered[:contentStart] + options.String() + rendered[contentEnd:]
		cursor = contentStart + options.Len() + len("</select>")
	}
}

func setDocumentLocale(source string, option uiLocaleOption) (string, error) {
	start := strings.Index(source, "<html")
	if start < 0 {
		return "", fmt.Errorf("HTML document has no html element")
	}
	endRelative := strings.IndexByte(source[start:], '>')
	if endRelative < 0 {
		return "", fmt.Errorf("HTML document has an unterminated html element")
	}
	end := start + endRelative
	tag := source[start : end+1]
	updated, err := setHTMLAttribute(tag, "lang", string(option.Code), false)
	if err != nil {
		return "", err
	}
	updated, err = setHTMLAttribute(updated, "dir", option.Dir, false)
	if err != nil {
		return "", err
	}
	return source[:start] + updated + source[end+1:], nil
}

func setHTMLAttribute(tag, name, value string, requireExisting bool) (string, error) {
	attributeStart, valueStart, valueEnd, found := findHTMLAttribute(tag, name)
	if found {
		_ = attributeStart
		return tag[:valueStart] + html.EscapeString(value) + tag[valueEnd:], nil
	}
	if requireExisting {
		return "", fmt.Errorf("required %s attribute is missing", name)
	}
	insert := strings.LastIndex(tag, ">")
	if insert < 0 {
		return "", fmt.Errorf("cannot add %s to unterminated tag", name)
	}
	return tag[:insert] + ` ` + name + `="` + html.EscapeString(value) + `"` + tag[insert:], nil
}

func findHTMLAttribute(tag, name string) (attributeStart, valueStart, valueEnd int, found bool) {
	needle := name + `="`
	cursor := 0
	for {
		relative := strings.Index(tag[cursor:], needle)
		if relative < 0 {
			return 0, 0, 0, false
		}
		start := cursor + relative
		if start == 0 || unicode.IsSpace(rune(tag[start-1])) || tag[start-1] == '<' {
			valueStart = start + len(needle)
			endRelative := strings.IndexByte(tag[valueStart:], '"')
			if endRelative < 0 {
				return 0, 0, 0, false
			}
			return start, valueStart, valueStart + endRelative, true
		}
		cursor = start + len(needle)
	}
}

func openingTagName(tag string) (string, error) {
	trimmed := strings.TrimSpace(tag)
	if !strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "</") {
		return "", fmt.Errorf("invalid opening tag")
	}
	trimmed = strings.TrimLeftFunc(trimmed[1:], unicode.IsSpace)
	end := 0
	for end < len(trimmed) {
		ch := trimmed[end]
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '-' {
			break
		}
		end++
	}
	if end == 0 {
		return "", fmt.Errorf("opening tag name is missing")
	}
	return strings.ToLower(trimmed[:end]), nil
}

func stripLocalizationMarkers(source string) string {
	markers := []string{"data-i18n", "data-i18n-placeholder", "data-i18n-title", "data-i18n-aria"}
	result := source
	for _, marker := range markers {
		for {
			start, _, end, found := findHTMLAttribute(result, marker)
			if !found {
				break
			}
			removeStart := start
			if removeStart > 0 && unicode.IsSpace(rune(result[removeStart-1])) {
				removeStart--
			}
			result = result[:removeStart] + result[end+1:]
		}
	}
	result = strings.ReplaceAll(result, " data-ui-locale-select", "")
	return result
}
