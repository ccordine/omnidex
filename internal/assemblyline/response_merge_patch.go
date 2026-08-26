package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/gryph/omnidex/internal/exactjson"
)

func responseCorrectionSchema(original PortableJob, targetField string) (map[string]any, error) {
	if original.Kind == WorkResponseCorrection {
		return nil, fmt.Errorf("response correction cannot wrap another response correction")
	}
	if original.Kind == WorkRepositoryRequirements {
		return nil, fmt.Errorf("aggregate requirement interpretation is not response-correctable")
	}
	if original.Kind == WorkApplicationJobSpecification {
		return applicationJobSpecificationResponseCorrectionSchema(targetField)
	}
	_, originalSchema, err := RenderPortableJob(original)
	if err != nil {
		return nil, err
	}
	if originalSchema == nil {
		return nil, fmt.Errorf("response correction requires a structured original response")
	}
	properties, ok := originalSchema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return nil, fmt.Errorf("response correction original schema has no properties")
	}
	mutable := make(map[string]any)
	for name, definition := range properties {
		fields, _ := definition.(map[string]any)
		if _, fixed := fields["const"]; fixed {
			continue
		}
		mutable[name] = definition
	}
	if len(mutable) == 0 {
		return nil, fmt.Errorf("response correction original schema has no mutable field")
	}
	if targetField != "" {
		return nil, fmt.Errorf("field-scoped response correction is unsupported for %s", original.Kind)
	} else if len(mutable) != 1 {
		return nil, fmt.Errorf(
			"response correction requires exactly one code-owned mutable semantic field; %s exposes %d",
			original.Kind, len(mutable),
		)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           mutable,
		"additionalProperties": false,
		"minProperties":        1,
		"maxProperties":        1,
	}, nil
}

func ApplyResponseCorrection(
	original PortableJob,
	retainedCandidate string,
	mergePatch string,
) (string, error) {
	return applyResponseCorrection(original, retainedCandidate, mergePatch, "")
}

func ApplyResponseCorrectionForField(
	original PortableJob,
	retainedCandidate string,
	mergePatch string,
	targetField string,
) (string, error) {
	return applyResponseCorrection(
		original, retainedCandidate, mergePatch, targetField,
	)
}

func applyResponseCorrection(
	original PortableJob,
	retainedCandidate string,
	mergePatch string,
	targetField string,
) (string, error) {
	if original.Kind == WorkApplicationJobSpecification {
		return applyApplicationJobSpecificationResponseCorrection(
			original, retainedCandidate, mergePatch, targetField,
		)
	}
	schema, err := responseCorrectionSchema(original, targetField)
	if err != nil {
		return "", err
	}
	retained, err := decodeJSONObject(retainedCandidate, "retained semantic candidate")
	if err != nil {
		return "", err
	}
	patch, err := decodeJSONObject(mergePatch, "semantic response merge patch")
	if err != nil {
		return "", err
	}
	if len(patch) != 1 {
		return "", fmt.Errorf("semantic response merge patch must contain exactly one top-level field")
	}
	properties := schema["properties"].(map[string]any)
	var field string
	var patchValue any
	for field, patchValue = range patch {
	}
	if _, allowed := properties[field]; !allowed {
		return "", fmt.Errorf("semantic response merge patch field %q is immutable or unsupported", field)
	}
	before := retained[field]
	retained[field] = mergeJSONValue(before, patchValue)
	changes := countJSONLeafChanges(before, retained[field])
	if changes != 1 {
		return "", fmt.Errorf(
			"semantic response merge patch changed %d JSON leaves; exactly one is required", changes,
		)
	}
	raw, err := json.Marshal(retained)
	if err != nil {
		return "", fmt.Errorf("encode corrected semantic response: %w", err)
	}
	return string(raw), nil
}

func decodeJSONObject(raw string, label string) (map[string]any, error) {
	if err := exactjson.ValidateUniqueObject([]byte(raw), label); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	if value == nil {
		return nil, fmt.Errorf("%s must be one JSON object", label)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode %s: trailing JSON value", label)
		}
		return nil, fmt.Errorf("decode %s trailing data: %w", label, err)
	}
	return value, nil
}

func mergeJSONValue(current any, patch any) any {
	patchObject, patchIsObject := patch.(map[string]any)
	if !patchIsObject {
		return patch
	}
	currentObject, currentIsObject := current.(map[string]any)
	if !currentIsObject {
		currentObject = map[string]any{}
	}
	merged := make(map[string]any, len(currentObject)+len(patchObject))
	for key, value := range currentObject {
		merged[key] = value
	}
	for key, value := range patchObject {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = mergeJSONValue(merged[key], value)
	}
	return merged
}

func countJSONLeafChanges(before any, after any) int {
	if reflect.DeepEqual(before, after) {
		return 0
	}
	beforeMap, beforeMapOK := before.(map[string]any)
	afterMap, afterMapOK := after.(map[string]any)
	if beforeMapOK && afterMapOK {
		keys := make(map[string]struct{}, len(beforeMap)+len(afterMap))
		for key := range beforeMap {
			keys[key] = struct{}{}
		}
		for key := range afterMap {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		changes := 0
		for _, key := range ordered {
			left, leftExists := beforeMap[key]
			right, rightExists := afterMap[key]
			if !leftExists {
				changes += countJSONLeaves(right)
			} else if !rightExists {
				changes += countJSONLeaves(left)
			} else {
				changes += countJSONLeafChanges(left, right)
			}
		}
		return changes
	}
	beforeSlice, beforeSliceOK := before.([]any)
	afterSlice, afterSliceOK := after.([]any)
	if beforeSliceOK && afterSliceOK {
		changes := 0
		shared := len(beforeSlice)
		if len(afterSlice) < shared {
			shared = len(afterSlice)
		}
		for index := 0; index < shared; index++ {
			changes += countJSONLeafChanges(beforeSlice[index], afterSlice[index])
		}
		for _, value := range beforeSlice[shared:] {
			changes += countJSONLeaves(value)
		}
		for _, value := range afterSlice[shared:] {
			changes += countJSONLeaves(value)
		}
		return changes
	}
	leftLeaves := countJSONLeaves(before)
	rightLeaves := countJSONLeaves(after)
	if rightLeaves > leftLeaves {
		return rightLeaves
	}
	return leftLeaves
}

func countJSONLeaves(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		count := 0
		for _, child := range typed {
			count += countJSONLeaves(child)
		}
		if count == 0 {
			return 1
		}
		return count
	case []any:
		count := 0
		for _, child := range typed {
			count += countJSONLeaves(child)
		}
		if count == 0 {
			return 1
		}
		return count
	default:
		return 1
	}
}
