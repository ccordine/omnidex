package userintent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// DecodeState lets a consumer validate its own typed DTO without accepting
// unknown fields, floating-point coercion, or trailing data.
func DecodeState[T any](raw json.RawMessage) (T, error) {
	var state T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&state); err != nil {
		return state, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return state, fmt.Errorf("trailing state data")
	}
	return state, nil
}

func applyPatch(snapshot json.RawMessage, patches []Patch, writable func(string) bool) (json.RawMessage, error) {
	after, err := applyPatchOperations(snapshot, patches, writable)
	if err != nil {
		return nil, err
	}
	if err := validateArrayDeltas(snapshot, after, patches); err != nil {
		return nil, err
	}
	// A group of edits may restore a whole subtree even when no pair does.
	// Replay without each touched subtree so unrelated intended edits cannot
	// conceal a redundant remove/replace/append sequence.
	groups := map[string]bool{}
	for _, patch := range patches {
		parts := strings.Split(patch.Path, "/")
		for i := 2; i <= len(parts); i++ {
			groups[strings.Join(parts[:i], "/")] = true
		}
	}
	for path := range groups {
		reduced := make([]Patch, 0, len(patches))
		for _, patch := range patches {
			if patch.Path != path && !strings.HasPrefix(patch.Path, path+"/") {
				reduced = append(reduced, patch)
			}
		}
		candidate, replayErr := applyPatchOperations(snapshot, reduced, writable)
		if replayErr == nil && bytes.Equal(candidate, after) {
			return nil, fmt.Errorf("patch contains redundant subtree changes")
		}
	}
	// Array indices shift as operations execute. Different pointers can therefore
	// remove and restore the same item. Reject such pairs only when replay proves
	// that omitting them preserves the entire intended result.
	for i, removed := range patches {
		if removed.Op != "remove" {
			continue
		}
		for j, added := range patches {
			if added.Op != "add" || i == j {
				continue
			}
			reduced := make([]Patch, 0, len(patches)-2)
			for k, patch := range patches {
				if k != i && k != j {
					reduced = append(reduced, patch)
				}
			}
			candidate, replayErr := applyPatchOperations(snapshot, reduced, writable)
			if replayErr == nil && bytes.Equal(candidate, after) {
				return nil, fmt.Errorf("patch contains redundant removal and addition")
			}
		}
	}
	return after, nil
}

func applyPatchOperations(snapshot json.RawMessage, patches []Patch, writable func(string) bool) (json.RawMessage, error) {
	document, err := DecodeState[map[string]any](snapshot)
	if err != nil {
		return nil, err
	}
	seen := map[string]Patch{}
	for _, patch := range patches {
		if !writable(patch.Path) {
			return nil, fmt.Errorf("state field is not writable: %s", patch.Path)
		}
		parts, err := pointerParts(patch.Path)
		if err != nil {
			return nil, err
		}
		// Ancestor/descendant edits could conceal redundant or overwritten changes.
		for path := range seen {
			// Inserting at a numeric pointer shifts the existing element away.
			insertion := patch.Op == "add" && isArrayElement(document, parts) &&
				(path == patch.Path || strings.HasPrefix(path, patch.Path+"/"))
			if insertion {
				continue
			}
			if path == patch.Path || strings.HasPrefix(path, patch.Path+"/") || strings.HasPrefix(patch.Path, path+"/") {
				return nil, fmt.Errorf("overlapping patch paths")
			}
		}
		// Keep prior edits attached to their array item as indices shift.
		rebased := map[string]Patch{}
		for path, previous := range seen {
			if next, keep := shiftedPath(path, patch, document); keep {
				rebased[next] = previous
			}
		}
		if patch.Op != "remove" || !isArrayElement(document, parts) {
			recordPath := patch.Path
			if patch.Op == "add" && parts[len(parts)-1] == "-" {
				if array, ok := arrayAt(document, parts[:len(parts)-1]); ok {
					// An append owns the inserted item at its concrete index.
					// Later numeric edits must overlap it just like numeric inserts.
					recordPath = patch.Path[:len(patch.Path)-1] + strconv.Itoa(len(array))
				}
			}
			rebased[recordPath] = patch
		}
		seen = rebased
		updated, err := patchNode(document, parts, patch)
		if err != nil {
			return nil, err
		}
		document = updated.(map[string]any)
	}
	return json.Marshal(document)
}

func isArrayElement(node any, parts []string) bool {
	for _, key := range parts[:len(parts)-1] {
		switch parent := node.(type) {
		case map[string]any:
			node = parent[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(parent) {
				return false
			}
			node = parent[index]
		default:
			return false
		}
	}
	_, array := node.([]any)
	return array
}

func pointerParts(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("patch requires an absolute field pointer")
	}
	parts := strings.Split(path[1:], "/")
	for i, part := range parts {
		for n := 0; n < len(part); n++ {
			if part[n] == '~' {
				n++
				if n == len(part) || (part[n] != '0' && part[n] != '1') {
					return nil, fmt.Errorf("invalid pointer escape")
				}
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}
func patchNode(node any, parts []string, patch Patch) (any, error) {
	key := parts[0]
	switch object := node.(type) {
	case map[string]any:
		old, exists := object[key]
		if len(parts) > 1 {
			if !exists {
				return nil, fmt.Errorf("patch parent is absent")
			}
			value, err := patchNode(old, parts[1:], patch)
			if err != nil {
				return nil, err
			}
			object[key] = value
			return object, nil
		}
		value, err := patchValue(old, exists, patch)
		if err != nil {
			return nil, err
		}
		if patch.Op == "remove" {
			delete(object, key)
		} else {
			object[key] = value
		}
		return object, nil
	case []any:
		index, err := strconv.Atoi(key)
		if key == "-" && patch.Op == "add" && len(parts) == 1 {
			index = len(object)
			err = nil
		}
		if err != nil || index < 0 || index > len(object) || (key != "-" && strconv.Itoa(index) != key) {
			return nil, fmt.Errorf("invalid array index")
		}
		if len(parts) > 1 {
			if index == len(object) {
				return nil, fmt.Errorf("array parent is absent")
			}
			value, err := patchNode(object[index], parts[1:], patch)
			if err != nil {
				return nil, err
			}
			object[index] = value
			return object, nil
		}
		exists := index < len(object)
		var old any
		if exists {
			old = object[index]
		}
		// add inserts into an array, including before an equal existing value.
		value, err := patchValue(old, exists && patch.Op != "add", patch)
		if err != nil {
			return nil, err
		}
		switch patch.Op {
		case "add":
			object = append(object, nil)
			copy(object[index+1:], object[index:])
			object[index] = value
		case "replace":
			object[index] = value
		case "remove":
			object = append(object[:index], object[index+1:]...)
		}
		return object, nil
	default:
		return nil, fmt.Errorf("patch parent is not an object or array")
	}
}
func patchValue(old any, exists bool, patch Patch) (any, error) {
	if patch.Op != "add" && !exists {
		return nil, fmt.Errorf("patch target is absent")
	}
	if patch.Op == "remove" {
		if len(patch.Value) != 0 {
			return nil, fmt.Errorf("remove cannot contain a value")
		}
		return nil, nil
	}
	if patch.Op != "add" && patch.Op != "replace" {
		return nil, fmt.Errorf("unsupported patch operation")
	}
	value, err := DecodeState[any](patch.Value)
	if err != nil {
		return nil, err
	}
	if exists && reflect.DeepEqual(old, value) {
		return nil, fmt.Errorf("patch contains unchanged state")
	}
	return value, nil
}

// Apply checks a proposed delta without mutating the supplied snapshot. Consumers
// must still validate their typed state and persist it under their own authority.
func Apply(snapshot json.RawMessage, patches []Patch, writable func(string) bool) (json.RawMessage, error) {
	if writable == nil || patches == nil {
		return nil, fmt.Errorf("invalid state patch contract")
	}
	return applyPatch(snapshot, patches, writable)
}
