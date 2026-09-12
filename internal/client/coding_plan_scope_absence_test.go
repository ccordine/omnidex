package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanClientRejectsRemovedScopeFields(t *testing.T) {
	for _, field := range []string{"scope_mode", "annotation"} {
		t.Run(field, func(t *testing.T) {
			plan := codingPlanFixture(t, 91, 1, 1, model.CodingPlanStateReview)
			raw, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if field == "scope_mode" {
				response[field] = json.RawMessage(`"expansive"`)
			} else {
				var leaves []map[string]json.RawMessage
				if err := json.Unmarshal(response["leaves"], &leaves); err != nil {
					t.Fatal(err)
				}
				leaves[0][field] = json.RawMessage(`"speculative_review"`)
				response["leaves"], err = json.Marshal(leaves)
				if err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writeJSON(t, writer, http.StatusOK, response)
			}))
			defer server.Close()
			_, err = testClient(t, server.URL).CodingPlan(
				context.Background(), testCLIChannel("/tmp/removed-scope-fields"), testWorkspaceIdentity, 91,
			)
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("removed coding-plan field %s: %v", field, err)
			}
		})
	}
}
