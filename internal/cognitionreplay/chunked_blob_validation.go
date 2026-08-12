package cognitionreplay

import (
	"fmt"
	"reflect"
)

func validateChunkedBlobBindings(
	values []ChunkedBlobBinding,
	manifestValues []ChunkedBlobBinding,
	expectedRole ChunkedBlobRole,
) error {
	if values == nil || manifestValues == nil || !reflect.DeepEqual(values, manifestValues) {
		return fmt.Errorf("chunked replay blob index changed")
	}
	if len(values) > maxSources {
		return fmt.Errorf("chunked replay blob binding count exceeds its bound")
	}
	previous := ""
	for index, binding := range values {
		if binding.Validate() != nil ||
			(previous != "" && binding.Manifest.SHA256 <= previous) {
			return fmt.Errorf("chunked replay blob binding %d is invalid, duplicated, or reordered", index+1)
		}
		previous = binding.Manifest.SHA256
	}
	if !validChunkedBlobRole(expectedRole) {
		return fmt.Errorf("chunked replay blob role authority is invalid")
	}
	return nil
}
