package worker

import (
	"fmt"
	"strconv"
	"strings"
)

func directCodingBrowserIntrinsicTag(tag string) bool {
	if tag == "" || strings.Contains(tag, "-") {
		return false
	}
	for index, char := range tag {
		if (char < 'a' || char > 'z') && (index == 0 || char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func directCodingBrowserSupportedIntrinsicTag(tag string) bool {
	switch tag {
	case "article", "aside", "blockquote", "br", "button", "caption", "code",
		"dd", "div", "dl", "dt", "em", "fieldset", "figcaption", "figure",
		"footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header",
		"hr", "input", "label", "legend", "li", "main", "nav", "ol",
		"optgroup", "option", "output", "p", "pre", "section", "select",
		"small", "span", "strong", "table", "tbody", "td", "textarea",
		"tfoot", "th", "thead", "time", "tr", "ul":
		return true
	default:
		return false
	}
}

func directCodingBrowserIntrinsicControl(
	tag string,
	attributes map[string]directCodingBrowserJSXAttribute,
) (string, string, bool, error) {
	switch tag {
	case "button":
		return "button", "action", true, nil
	case "textarea":
		return "textbox", "text", true, nil
	case "select":
		if attributes["multiple"].present {
			return "listbox", "selection", true, nil
		}
		if size := attributes["size"]; size.present {
			value, err := strconv.Atoi(size.literal)
			if err != nil || value < 1 {
				return "", "", false, fmt.Errorf(
					"browser public surface select size is not a positive integer",
				)
			}
			if value > 1 {
				return "listbox", "selection", true, nil
			}
		}
		return "combobox", "selection", true, nil
	case "input":
		return directCodingBrowserIntrinsicInput(attributes)
	default:
		return "", "", false, nil
	}
}

func directCodingBrowserIntrinsicInput(
	attributes map[string]directCodingBrowserJSXAttribute,
) (string, string, bool, error) {
	if attributes["list"].present {
		return "", "", false, fmt.Errorf(
			"browser public surface rejects role-affecting input list",
		)
	}
	inputType := "text"
	if attribute := attributes["type"]; attribute.present {
		inputType = strings.ToLower(strings.TrimSpace(attribute.literal))
	}
	switch inputType {
	case "hidden":
		return "", "", false, nil
	case "text", "email", "tel", "url":
		return "textbox", "text", true, nil
	case "search":
		return "searchbox", "text", true, nil
	case "number":
		return "spinbutton", "number", true, nil
	case "range":
		return "slider", "number", true, nil
	case "checkbox":
		return "checkbox", "boolean", true, nil
	case "radio":
		return "radio", "selection", true, nil
	default:
		return "", "", false, fmt.Errorf(
			"browser public surface rejects unsupported input type %q", inputType,
		)
	}
}
