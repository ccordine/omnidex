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
	payload := dataSourcePublic(queue.DataSourceRecord{
		ID: "source-1", ReadOnly: true, Host: "database.internal", Username: "reader",
		AuthorityURL: "https://host.internal", CredentialEnv: "HOST_TOKEN",
	})
	if readOnly, ok := payload["read_only"].(bool); !ok || !readOnly {
		t.Fatalf("public read-only invariant=%v", payload["read_only"])
	}
	for _, forbidden := range []string{
		"domain", "context_prompt", "privacy_mode", "password", "password_set", "password_hint",
		"dsn", "use_dsn", "host", "port", "database_name", "username", "ssl_mode",
		"authority_url", "credential_env",
	} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("public data-source projection exposes %q", forbidden)
		}
	}
}

func TestDecodeDataSourceUpsertRequiresExplicitExecutionConfigurationFields(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/data-sources", strings.NewReader(`{
		"name":"Clinical host","driver":"postgres","execution_mode":"delegated",
		"host":"","port":0,"database_name":"","username":"","password":"",
		"ssl_mode":"","use_dsn":false,"dsn":"",
		"authority_url":"https://application.internal","credential_env":"OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN"
	}`))
	input, err := decodeDataSourceUpsert(request)
	if err != nil {
		t.Fatal(err)
	}
	if input.ExecutionMode != "delegated" || input.AuthorityURL != "https://application.internal" ||
		input.CredentialEnv != "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN" {
		t.Fatalf("decoded delegated authority=%+v", input)
	}
}
