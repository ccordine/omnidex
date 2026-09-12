package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestUnrequestedCandidatesCannotConsumeRetainedCapacityOrSemanticWork(t *testing.T) {
	for _, request := range []string{
		"The finished software lets a reader bookmark a passage.",
		"The finished software lets an operator silence an alarm.",
	} {
		for _, includeRequested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/requested=%t", request, includeRequested), func(t *testing.T) {
				applicationContext, err := assemblyline.BootstrapApplicationContext(request)
				if err != nil {
					t.Fatal(err)
				}
				candidates := make([]string, assemblyline.MaxApplicationRequirementLeaves)
				for index := range candidates {
					candidates[index] = fmt.Sprintf("The finished software sends an unrelated report to recipient %d.", index+1)
				}
				if includeRequested {
					candidates = append(candidates, request)
				}
				authorizations, downstream, inventories := 0, 0, 0
				runtime := typedWorkerRuntime{Context: context.Background(), Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					var response string
					switch job.Kind {
					case assemblyline.WorkApplicationRequirementInventory:
						inventories++
						response = strings.Join(candidates, "\n")
					case assemblyline.WorkApplicationRequirementCandidateAuthorization:
						var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
						if err := json.Unmarshal(job.Payload, &input); err != nil {
							return assemblyline.PortableResult{}, err
						}
						if input.Candidate == request {
							t.Fatal("exact request content required semantic authorization")
						}
						authorizations++
						response = "B"
					case assemblyline.WorkApplicationRequirementCandidateKind:
						var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
						if err := json.Unmarshal(job.Payload, &input); err != nil {
							return assemblyline.PortableResult{}, err
						}
						if !includeRequested || input.Candidate != request {
							t.Fatalf("unauthorized candidate reached classification: %+v", input)
						}
						downstream++
						response = "A"
						if input.Dimension == assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension {
							response = "B"
						}
					case assemblyline.WorkApplicationRequirementCandidateCardinality:
						downstream++
						response = "A"
					case assemblyline.WorkApplicationRequirementCandidateResultRelation:
						downstream++
						response = "B"
					default:
						return assemblyline.PortableResult{}, fmt.Errorf("unauthorized candidate created additional semantic work %q", job.Kind)
					}
					return assemblyline.PortableResult{Candidate: response}, nil
				}}
				proposals, err := resolveDirectCodingApplicationPlan(runtime,
					directCodingApplicationIntentModels{Requirements: "fixture-requirement", ResultRelation: "fixture-result"},
					assemblyline.ApplicationIntentInput{UserRequest: request, Context: applicationContext}, nil)
				if err != nil {
					t.Fatal(err)
				}
				wantProposals, wantDownstream := 0, 0
				if includeRequested {
					wantProposals, wantDownstream = 1, 4
				}
				if inventories != 1 || authorizations != assemblyline.MaxApplicationRequirementLeaves || downstream != wantDownstream || len(proposals) != wantProposals {
					t.Fatalf("rejected inventory changed retained capacity: inventories=%d authorizations=%d downstream=%d proposals=%+v", inventories, authorizations, downstream, proposals)
				}
				if includeRequested && proposals[0].Statement != request {
					t.Fatalf("requested outcome lost: %+v", proposals)
				}
			})
		}
	}
}
