package userintent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Direct array edits must not encode a longer sequence than the resulting
// insert/remove/replace delta. Nested field edits retain their own granularity.
func validateArrayDeltas(before, after json.RawMessage, patches []Patch) error {
	old, err := DecodeState[map[string]any](before)
	if err != nil {
		return err
	}
	next, err := DecodeState[map[string]any](after)
	if err != nil {
		return err
	}
	type delta struct {
		before []any
		count  int
	}
	groups := map[string]delta{}
	for _, patch := range patches {
		parent := patch.Path[:strings.LastIndex(patch.Path, "/")]
		if parent != "" {
			parts, err := pointerParts(parent)
			if err != nil {
				return err
			}
			if array, ok := arrayAt(old, parts); ok {
				group, exists := groups[parent]
				if !exists {
					// patchNode mutates arrays in place; retain an independent baseline.
					raw, _ := json.Marshal(array)
					group.before, err = DecodeState[[]any](raw)
					if err != nil {
						return err
					}
				}
				group.count++
				groups[parent] = group
			}
		}
		rebased := map[string]delta{}
		for path, group := range groups {
			if path, keep := shiftedPath(path, patch, old); keep {
				rebased[path] = group
			}
		}
		groups = rebased
		parts, err := pointerParts(patch.Path)
		if err != nil {
			return err
		}
		updated, err := patchNode(old, parts, patch)
		if err != nil {
			return err
		}
		old = updated.(map[string]any)
	}
	for path, group := range groups {
		parts, _ := pointerParts(path)
		right, ok := arrayAt(next, parts)
		if ok && arrayEditDistance(group.before, right, group.count) < group.count {
			return fmt.Errorf("patch contains redundant array edits at %s", path)
		}
	}
	return nil
}

func arrayAt(node any, parts []string) ([]any, bool) {
	for _, key := range parts {
		switch parent := node.(type) {
		case map[string]any:
			node = parent[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(parent) {
				return nil, false
			}
			node = parent[index]
		default:
			return nil, false
		}
	}
	array, ok := node.([]any)
	return array, ok
}

// Only distances below limit matter. A diagonal band bounds work to the
// operation budget times array length, rather than the square of its length.
func arrayEditDistance(left, right []any, limit int) int {
	previous := map[int]int{}
	for j := 0; j <= min(len(right), limit); j++ {
		previous[j] = j
	}
	value := func(row map[int]int, j int) int {
		if v, ok := row[j]; ok {
			return v
		}
		return limit
	}
	for i := 1; i <= len(left); i++ {
		current := map[int]int{}
		for j := max(0, i-limit); j <= min(len(right), i+limit); j++ {
			if j == 0 {
				current[j] = i
				continue
			}
			replacement := 1
			if reflect.DeepEqual(left[i-1], right[j-1]) {
				replacement = 0
			}
			current[j] = min(limit, value(previous, j)+1, value(current, j-1)+1, value(previous, j-1)+replacement)
		}
		previous = current
	}
	return value(previous, len(right))
}

// shiftedPath follows an existing location through a structural array edit.
// A removed item's locations no longer refer to any surviving state.
func shiftedPath(path string, patch Patch, document any) (string, bool) {
	parts, _ := pointerParts(patch.Path)
	if (patch.Op != "add" && patch.Op != "remove") || !isArrayElement(document, parts) {
		return path, true
	}
	index, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return path, true
	} // append leaves existing indices intact
	parent := patch.Path[:strings.LastIndex(patch.Path, "/")+1]
	if !strings.HasPrefix(path, parent) {
		return path, true
	}
	suffix := strings.TrimPrefix(path, parent)
	key, rest, nested := strings.Cut(suffix, "/")
	position, err := strconv.Atoi(key)
	if err != nil || position < index {
		return path, true
	}
	if patch.Op == "remove" {
		if position == index {
			return "", false
		}
		position--
	} else {
		position++
	}
	result := parent + strconv.Itoa(position)
	if nested {
		result += "/" + rest
	}
	return result, true
}
