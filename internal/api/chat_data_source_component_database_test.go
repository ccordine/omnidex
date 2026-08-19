package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestChatDataSourceOptionsEndpointProjectsOnlyOpaqueIdentity(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundleThroughPrefix(t, "115")); err != nil {
		t.Fatal(err)
	}
	source, err := repository.CreateDataSource(t.Context(), queue.DataSourceUpsert{
		Name: "<Customer & Evidence>", Driver: "postgres",
		Host: "private.internal", Port: 5432, DatabaseName: "secret_database",
		Username: "secret_reader", Password: "secret_password", SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(nil, nil)
	server.repo = repository
	server.channelStore = repository
	server.mux = http.NewServeMux()
	server.routes()
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/chat/data-sources?limit=1&offset=0", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var page chatComponentPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`value="` + source.ID + `"`, `&lt;Customer &amp; Evidence&gt;`,
		`data-recyclr-target="new-channel-data-source-options"`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("chat data-source page lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
	for _, forbidden := range []string{
		"private.internal", "secret_database", "secret_reader", "secret_password", "require",
	} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("chat data-source endpoint leaked %q: %s", forbidden, response.Body.String())
		}
	}
}
