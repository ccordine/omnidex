package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeDataSourceUpsertRejectsRetiredAndServerOwnedControls(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"domain", "context_prompt", "privacy_mode", "read_only"} {
		t.Run(field, func(t *testing.T) {
			body := `{"name":"Exact","driver":"postgres","host":"localhost","port":5432,` +
				`"database_name":"app","username":"reader","ssl_mode":"prefer","` + field + `":false}`
			request := httptest.NewRequest(http.MethodPost, "/v1/admin/data-sources", strings.NewReader(body))
			if _, err := decodeDataSourceUpsert(request); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("retired/server-owned field %q error=%v", field, err)
			}
		})
	}
}

func TestDataSourcePublicProjectionKeepsInvariantNotMutableProfileControls(t *testing.T) {
	t.Parallel()
	payload := dataSourcePublic(queue.DataSourceRecord{ID: "source-1", ReadOnly: true})
	if readOnly, ok := payload["read_only"].(bool); !ok || !readOnly {
		t.Fatalf("public read-only invariant=%v", payload["read_only"])
	}
	for _, forbidden := range []string{"domain", "context_prompt", "privacy_mode", "password", "dsn"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("public data-source projection exposes %q", forbidden)
		}
	}
}
