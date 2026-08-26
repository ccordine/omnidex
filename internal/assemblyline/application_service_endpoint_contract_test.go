package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceEndpointContractIsComposedAndValidatedByCode(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	contract, err := ComposeApplicationServiceEndpointContract(
		authority,
		ApplicationServiceEndpointExposureResult{
			Schema: ApplicationServiceEndpointExposureSchemaV1, Exposure: ApplicationServiceEndpointPublic,
		},
		ApplicationServiceEndpointMethodResult{
			Schema: ApplicationServiceEndpointMethodSchemaV1, Method: ApplicationServiceEndpointGET,
		},
		ApplicationServiceEndpointRouteTemplateResult{
			Schema: ApplicationServiceEndpointRouteTemplateSchemaV1, RouteTemplate: "/records/{record_id}",
		},
		ApplicationServiceEndpointRequestMediaResult{
			Schema: ApplicationServiceEndpointRequestMediaSchemaV1, RequestMedia: ApplicationServiceEndpointMediaNone,
		},
		ApplicationServiceEndpointResponseMediaResult{
			Schema: ApplicationServiceEndpointResponseMediaSchemaV1, ResponseMedia: ApplicationServiceEndpointJSON,
		},
		ApplicationServiceEndpointSuccessStatusResult{
			Schema: ApplicationServiceEndpointSuccessStatusSchemaV1, SuccessStatus: 200,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Schema != ApplicationServiceEndpointContractSchemaV1 ||
		contract.Method != ApplicationServiceEndpointGET ||
		contract.RouteTemplate != "/records/{record_id}" {
		t.Fatalf("composed contract=%+v", contract)
	}
}

func TestApplicationServiceEndpointContractRejectsInvalidAggregateCompatibility(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	valid := ApplicationServiceEndpointContract{
		Schema:   ApplicationServiceEndpointContractSchemaV1,
		Exposure: ApplicationServiceEndpointAuthenticated,
		Method:   ApplicationServiceEndpointPOST, RouteTemplate: "/records",
		RequestMedia:  ApplicationServiceEndpointJSON,
		ResponseMedia: ApplicationServiceEndpointJSON, SuccessStatus: 201,
	}
	if err := valid.ValidateFor(authority); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ApplicationServiceEndpointContract){
		"schema":   func(contract *ApplicationServiceEndpointContract) { contract.Schema = "v2" },
		"exposure": func(contract *ApplicationServiceEndpointContract) { contract.Exposure = "private" },
		"method":   func(contract *ApplicationServiceEndpointContract) { contract.Method = "TRACE" },
		"route":    func(contract *ApplicationServiceEndpointContract) { contract.RouteTemplate = "/Records" },
		"request media": func(contract *ApplicationServiceEndpointContract) {
			contract.RequestMedia = ApplicationServiceEndpointBinary
		},
		"response media": func(contract *ApplicationServiceEndpointContract) {
			contract.ResponseMedia = ApplicationServiceEndpointMultipart
		},
		"status": func(contract *ApplicationServiceEndpointContract) { contract.SuccessStatus = 418 },
		"payload with 204": func(contract *ApplicationServiceEndpointContract) {
			contract.SuccessStatus = 204
		},
	} {
		candidate := valid
		mutate(&candidate)
		if err := candidate.ValidateFor(authority); err == nil {
			t.Fatalf("aggregate contract accepted invalid %s: %+v", name, candidate)
		}
	}
	getWithBody := valid
	getWithBody.Method = ApplicationServiceEndpointGET
	getWithBody.SuccessStatus = 200
	if err := getWithBody.ValidateFor(authority); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("GET body compatibility error=%v", err)
	}
}

func testServiceEndpointTaskAuthority() ApplicationServiceEndpointTaskAuthority {
	return ApplicationServiceEndpointTaskAuthority{
		ProductContext:     "inventory service",
		RequirementQuote:   "Clients can retrieve an inventory record.",
		Objective:          "Expose retrieval of one inventory record.",
		RequiredBehaviors:  []string{"Return the requested record."},
		AcceptanceCriteria: []string{"A known identity returns its record."},
	}
}
