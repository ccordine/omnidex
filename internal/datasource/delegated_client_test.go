package datasource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDelegatedClientFetchesAuthorizedSchemaAndTypedEvidence(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	id := findColumn(t, orders, "id")
	plan, err := BuildRelationalQueryPlan(snapshot, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: orders.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: id.ID}}, Limit: 5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorityID := "dba_" + strings.Repeat("a", 64)
	token := "integration-token-that-never-enters-json"
	requests := 0
	transport := delegatedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("Authorization") != "Bearer "+token ||
			request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("delegated request headers=%v", request.Header)
			return delegatedJSONResponse(http.StatusUnauthorized, DelegatedErrorResponse{
				Schema: DelegatedErrorResponseV1, ErrorCode: "unauthorized",
			}), nil
		}
		switch request.URL.Path {
		case DelegatedSchemaPath:
			var input DelegatedSchemaRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if input.Schema != DelegatedSchemaRequestV1 || input.SourceID != snapshot.SourceID ||
				input.AuthorityID != authorityID {
				t.Errorf("schema request=%+v", input)
			}
			return delegatedJSONResponse(http.StatusOK, delegatedSchemaResponseFixture(t, snapshot)), nil
		case DelegatedEvidencePath:
			var input DelegatedEvidenceRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			encoded, _ := json.Marshal(input)
			visible := strings.ToLower(string(encoded))
			for _, forbidden := range []string{"select ", "password", "credential", token} {
				if strings.Contains(visible, strings.ToLower(forbidden)) {
					t.Errorf("delegated evidence request leaked %q: %s", forbidden, visible)
				}
			}
			if input.Schema != DelegatedEvidenceRequestV1 || input.AuthorityID != authorityID ||
				input.Snapshot.Fingerprint != snapshot.Fingerprint ||
				input.Plan.PlanHash != plan.PlanHash || input.Limits.MaxRows != 5 {
				t.Errorf("evidence request=%+v", input)
			}
			return delegatedJSONResponse(http.StatusOK, DelegatedEvidenceResponse{
				Schema:   DelegatedEvidenceResponseV1,
				Evidence: delegatedClientEvidenceFixture(t, plan, id.ID),
			}), nil
		default:
			return delegatedJSONResponse(http.StatusNotFound, DelegatedErrorResponse{
				Schema: DelegatedErrorResponseV1, ErrorCode: "not_found",
			}), nil
		}
	})
	client, err := NewDelegatedClient("https://host.internal", token, &http.Client{
		Timeout: 2 * time.Second, Transport: transport,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := client.FetchSchema(t.Context(), snapshot.SourceID, snapshot.SourceName, authorityID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint != snapshot.Fingerprint {
		t.Fatalf("loaded snapshot=%+v", loaded)
	}
	limits := DefaultExecutionLimits()
	limits.MaxRows = 5
	evidence, err := client.Execute(t.Context(), authorityID, snapshot, plan, limits)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || evidence.Result.Rows[0][0].Value != "41" {
		t.Fatalf("requests=%d evidence=%+v", requests, evidence)
	}
}

func delegatedSchemaResponseFixture(t *testing.T, snapshot SchemaSnapshot) DelegatedSchemaResponse {
	t.Helper()
	definitions, err := definitionsFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	relations := make([]DelegatedRelationDefinition, len(definitions))
	for index, definition := range definitions {
		relation := DelegatedRelationDefinition{
			Schema: definition.Schema, Name: definition.Name, Kind: definition.Kind,
			RowEstimate: definition.RowEstimate, PrimaryKeyName: definition.PrimaryKeyName,
			PrimaryKey: append([]string(nil), definition.PrimaryKey...),
		}
		for _, column := range definition.Columns {
			relation.Columns = append(relation.Columns, DelegatedColumnDefinition{
				Name: column.Name, Ordinal: column.Ordinal, DataType: column.DataType,
				TypeCategory: column.TypeCategory, Nullable: column.Nullable,
				Generated: column.Generated, Identity: column.Identity,
				AllowedValues: append([]string(nil), column.AllowedValues...),
			})
		}
		for _, foreignKey := range definition.ForeignKeys {
			relation.ForeignKeys = append(relation.ForeignKeys, DelegatedForeignKeyDefinition{
				Name: foreignKey.Name, Columns: append([]string(nil), foreignKey.Columns...),
				ReferencedSchema:   foreignKey.ReferencedSchema,
				ReferencedRelation: foreignKey.ReferencedRelation,
				ReferencedColumns:  append([]string(nil), foreignKey.ReferencedColumns...),
			})
		}
		relations[index] = relation
	}
	return DelegatedSchemaResponse{
		Schema: DelegatedSchemaResponseV1, SourceID: snapshot.SourceID,
		CapturedAt: snapshot.CapturedAt, Relations: relations,
	}
}

func TestDelegatedClientFailsClosedOnInvalidConfigurationAndHostEvidence(t *testing.T) {
	t.Parallel()
	for name, values := range map[string][2]string{
		"missing token": {"https://host.internal", ""},
		"URL query":     {"https://host.internal?scope=all", "long-enough-integration-token"},
		"URL user info": {"https://user@host.internal", "long-enough-integration-token"},
		"wrong scheme":  {"file:///tmp/host", "long-enough-integration-token"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDelegatedClient(values[0], values[1], &http.Client{Timeout: time.Second}); err == nil {
				t.Fatal("invalid delegated client configuration was accepted")
			}
		})
	}

	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	id := findColumn(t, orders, "id")
	plan, err := BuildRelationalQueryPlan(snapshot, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: orders.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: id.ID}}, Limit: 2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := delegatedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		evidence := delegatedClientEvidenceFixture(t, plan, id.ID)
		evidence.Result.Rows[0][0].Value = "forged"
		return delegatedJSONResponse(http.StatusOK, DelegatedEvidenceResponse{
			Schema: DelegatedEvidenceResponseV1, Evidence: evidence,
		}), nil
	})
	client, err := NewDelegatedClient(
		"https://host.internal", "long-enough-integration-token",
		&http.Client{Timeout: time.Second, Transport: transport},
	)
	if err != nil {
		t.Fatal(err)
	}
	limits := DefaultExecutionLimits()
	limits.MaxRows = 2
	if _, err := client.Execute(t.Context(), "dba_"+strings.Repeat("b", 64), snapshot, plan, limits); err == nil {
		t.Fatal("host-forged evidence was accepted")
	}
}

func TestDelegatedCredentialEnvironmentCannotSelectArbitraryProcessSecrets(t *testing.T) {
	t.Parallel()
	if err := ValidateDelegatedCredentialEnvironmentName("OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN"); err != nil {
		t.Fatal(err)
	}
	urlEnvironment, err := DelegatedAuthorityURLEnvironmentName("OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN")
	if err != nil || urlEnvironment != "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_URL" {
		t.Fatalf("URL environment=%q error=%v", urlEnvironment, err)
	}
	for _, value := range []string{
		"OPENAI_API_KEY", "DATABASE_URL", "OMNIDEX_DELEGATED_AUTHORITY_",
		"OMNIDEX_DELEGATED_AUTHORITY_APPLICATION", "OMNIDEX_DELEGATED_AUTHORITY_mixed_TOKEN",
		"OMNIDEX_DELEGATED_AUTHORITY_" + strings.Repeat("A", 95) + "_TOKEN",
	} {
		if err := ValidateDelegatedCredentialEnvironmentName(value); err == nil {
			t.Fatalf("arbitrary credential environment %q was accepted", value)
		}
	}
}

type delegatedRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip delegatedRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func delegatedJSONResponse(status int, body any) *http.Response {
	encoded, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(encoded)),
	}
}

func delegatedClientEvidenceFixture(t *testing.T, plan RelationalQueryPlan, fieldID string) EvidenceResult {
	t.Helper()
	columns := []EvidenceColumn{{
		Name: "c1", PostgresTypeOID: postgresOIDInt8, FieldID: fieldID, TypeCategory: TypeInteger,
	}}
	rows := [][]EvidenceValue{{{Kind: EvidenceInteger, Value: "41"}}}
	columnBytes, _ := json.Marshal(columns)
	rowBytes, _ := json.Marshal(rows[0])
	result, err := finalizeTypedResult(columns, rows, len(columnBytes)+len(rowBytes))
	if err != nil {
		t.Fatal(err)
	}
	queryDigest := sha256.Sum256([]byte("host-authorized-query"))
	return EvidenceResult{
		Schema: EvidenceResultV1,
		Provenance: EvidenceProvenance{
			SourceID: plan.SourceID, SchemaFingerprint: plan.SchemaFingerprint,
			IntentHash: plan.IntentHash, QueryHash: hex.EncodeToString(queryDigest[:]),
			ResultHash: result.Hash, Plan: ExecutionPlan{TotalCost: 2, EstimatedRows: 1},
			AcquiredAt: time.Unix(1_700_000_100, 0).UTC(),
		},
		Result: result,
	}
}
