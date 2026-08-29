package api

import "net/url"

// oneQueryValue is used only after a boundary has rejected duplicate values.
func oneQueryValue(values url.Values, key string) (string, bool) {
	items, present := values[key]
	if !present || len(items) != 1 {
		return "", false
	}
	return items[0], true
}
