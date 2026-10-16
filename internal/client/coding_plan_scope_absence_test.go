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

func TestCodingPlanClientRejectsFieldsOutsideTheSharedTransportContract(t *testing.T) {
	for _, field := range []string{"scope_mode", "annotation", "repository_pagination"} {
		t.Run(field, func(t *testing.T) {
			plan := codingLedgerFixture(codingPlanFixture(t, 91, 1, 1, model.CodingPlanStateReview))
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
			} else if field == "annotation" {
				var ledger map[string]json.RawMessage
				if err := json.Unmarshal(response["ledger"], &ledger); err != nil {
					t.Fatal(err)
				}
				var constraints []map[string]json.RawMessage
				if err := json.Unmarshal(ledger["constraints"], &constraints); err != nil {
					t.Fatal(err)
				}
				constraints[0][field] = json.RawMessage(`"speculative_review"`)
				ledger["constraints"], err = json.Marshal(constraints)
				if err == nil {
					response["ledger"], err = json.Marshal(ledger)
				}
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var delta map[string]json.RawMessage
				if err := json.Unmarshal(response["latest_delta"], &delta); err != nil {
					t.Fatal(err)
				}
				delta["limit"] = json.RawMessage(`200`)
				response["latest_delta"], err = json.Marshal(delta)
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
			want := field
			if field == "repository_pagination" {
				want = "limit"
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("removed coding-plan field %s: %v", field, err)
			}
		})
	}
}
