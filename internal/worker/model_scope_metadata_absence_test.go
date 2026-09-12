package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelRoutingRejectsRemovedCodingScopeMetadata(t *testing.T) {
	for _, value := range []string{`"strict"`, `"normal"`, `"expansive"`, `null`} {
		_, err := modelRoutingFromJobMetadata(json.RawMessage(`{"model_config":{},"coding_scope_mode":` + value + `}`))
		if err == nil || !strings.Contains(err.Error(), "coding_scope_mode") {
			t.Fatalf("removed scope value %s: %v", value, err)
		}
	}
	if _, err := modelRoutingFromJobMetadata(json.RawMessage(`{"model_config":{}}`)); err != nil {
		t.Fatalf("model routing still requires an unrelated coding scope policy: %v", err)
	}
}
