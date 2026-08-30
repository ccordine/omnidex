package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	liveApplicationCandidateKindBoundaryScope = "live-application-candidate-inventory-boundary-v12"
)

func TestLiveApplicationRequirementCandidateInventoryBoundaryQualification(t *testing.T) {
	ctx, modelName, transport := newLiveApplicationSemanticBoundaryTransport(
		t,
		liveApplicationCandidateKindBoundaryScope,
	)
	fixtures := []struct {
		name, request  string
		expectsRuntime bool
		expectsDerived bool
	}{
		{
			name: "product-name-image-resizer", request: "Build an image resizer in Rust.",
			expectsRuntime: true, expectsDerived: true,
		},
		{
			name: "product-name-barcode-scanner", request: "Create a barcode scanner for a mobile browser.",
			expectsRuntime: true, expectsDerived: true,
		},
		{
			name: "direct-runtime-image-resize", request: "Resize the submitted image.",
			expectsRuntime: true, expectsDerived: true,
		},
		{
			name: "direct-runtime-word-sort", request: "Sort submitted words alphabetically.",
			expectsRuntime: true, expectsDerived: true,
		},
		{name: "no-runtime-react-shell", request: "Build a React application."},
		{name: "no-runtime-framework", request: "Use React."},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			projectionCase := liveCodingQualificationCase{
				name: fixture.name, request: fixture.request,
			}
			runtime := typedWorkerRuntime{
				Context: ctx, MaxAttempts: exactSemanticLeafCalls,
				Execute: func(
					job assemblyline.PortableJob,
					selectedModel string,
				) (assemblyline.PortableResult, error) {
					if selectedModel != modelName {
						return assemblyline.PortableResult{}, fmt.Errorf("selected model changed")
					}
					prompt, err := assemblyline.RenderPortableJob(job)
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					if err := validateLiveCodingQualificationProjection(
						projectionCase,
						job,
						prompt,
					); err != nil {
						return assemblyline.PortableResult{}, err
					}
					return transport.execute(ctx, job, selectedModel)
				},
			}
			start := transport.callCount()
			applicationContext, err := assemblyline.BootstrapApplicationContext(
				fixture.request,
				assemblyline.ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}

			resolution, err := resolveDirectCodingApplicationIntent(
				runtime,
				modelName,
				assemblyline.ApplicationIntentInput{
					UserRequest: fixture.request,
					Context:     applicationContext,
				},
				nil,
			)
			calls := transport.callsFrom(start)
			logLiveCodingQualification(t, fixture.name, modelName, "not-applicable", calls)
			if !fixture.expectsRuntime {
				if err == nil || !strings.Contains(
					err.Error(),
					"produced no retained task-local runtime outcome",
				) {
					t.Fatalf("no-runtime request error=%v", err)
				}
				counts := map[assemblyline.WorkKind]int{}
				for _, call := range calls {
					counts[call.kind]++
				}
				if counts[assemblyline.WorkApplicationProductContext] != 0 ||
					counts[assemblyline.WorkApplicationRequirementInventory] != 1 {
					t.Fatalf("no-runtime request bypassed ordinary intake: %+v", calls)
				}
				if len(calls) == 0 || calls[0].kind != assemblyline.WorkApplicationRequirementInventory {
					t.Fatalf("no-runtime request did not begin with inventory intake: %+v", calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(resolution.Requirements) == 0 {
				t.Fatal("runtime-bearing request produced no accepted requirement")
			}
			inventoryCalls := 0
			productCalls := 0
			authorizationCalls := 0
			explicitResultRelations := 0
			for _, call := range calls {
				switch call.kind {
				case assemblyline.WorkApplicationRequirementInventory:
					inventoryCalls++
				case assemblyline.WorkApplicationProductContext:
					productCalls++
				case assemblyline.WorkApplicationRequirementCandidateAuthorization:
					authorizationCalls++
				case assemblyline.WorkApplicationRequirementCandidateResultRelation:
					if call.candidate == assemblyline.ApplicationRequirementExplicitResultRelation {
						explicitResultRelations++
					}
				}
			}
			if inventoryCalls != 1 || productCalls != 1 ||
				len(calls) == 0 || calls[0].kind != assemblyline.WorkApplicationRequirementInventory ||
				calls[len(calls)-1].kind != assemblyline.WorkApplicationProductContext {
				t.Fatalf(
					"runtime intake order=%+v inventory calls=%d product calls=%d",
					calls,
					inventoryCalls,
					productCalls,
				)
			}
			if fixture.expectsDerived && authorizationCalls == 0 {
				t.Fatal("derived product-name candidates bypassed semantic authorization")
			}
			if fixture.expectsDerived && explicitResultRelations == 0 {
				t.Fatal("derived product-name candidates produced no explicit result relation")
			}
			for _, requirement := range resolution.Requirements {
				if fixture.expectsDerived && requirement.Statement == fixture.request {
					t.Fatalf("product-name wrapper survived as accepted leaf: %q", requirement.Statement)
				}
				if strings.TrimSpace(requirement.Statement) == "" {
					t.Fatal("accepted an empty runtime requirement")
				}
			}
		})
	}
}
