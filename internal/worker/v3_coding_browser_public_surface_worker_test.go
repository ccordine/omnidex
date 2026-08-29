package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestBrowserPublicSurfaceQueryRejectionIsTerminalBeforeFinalization(t *testing.T) {
	rawSurface := browserAcceptanceInventorySurface()
	portableSurface, err := directCodingBrowserPortablePublicInteractionSurface(rawSurface)
	if err != nil {
		t.Fatal(err)
	}
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.SourceBlock{
			ID:        "acceptance.inventory",
			Signature: "async function VerifyInventory(): Promise<void>",
			API:       "async function VerifyInventory(): Promise<void>",
			Contract:  "Verify the public inventory adjustment.",
			Globals:   []string{"expect", "screen"},
			TaskID:    "task_001", Role: assemblyline.SourceBlockTaskVerification,
		},
		dialect: "TypeScript 5.9.3 with TSX", tsx: true,
		publicInteractionSurface: &portableSurface,
		validateInitialCandidate: func(candidate string) error {
			return validateDirectCodingBrowserAcceptanceRoleQueries(
				candidate, true, rawSurface, assemblyline.ApplicationRequirementNoDerivedResult,
			)
		},
	}
	const candidate = `async function VerifyInventory(): Promise<void> {
  expect(screen.getByRole('textbox')).toBeInTheDocument();
}`
	executions := 0
	finalizations := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(scope, model, prompt string) (string, error) {
			executions++
			if scope != "portable_fragment_worker" || model != "initial" ||
				strings.Count(prompt, "PUBLIC_INTERACTION_SURFACE_V1") != 1 {
				t.Fatalf("verification worker lost public-surface receipt:\n%s", prompt)
			}
			return candidate, nil
		}),
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			finalizations++
			if validationErr == nil || !strings.Contains(
				validationErr.Error(), "matches 2 public controls",
			) {
				t.Fatalf("finalized public-query rejection=%v", validationErr)
			}
			var parserRepair *directCodingTypeScriptInitialFragmentRejection
			if errors.As(validationErr, &parserRepair) {
				t.Fatalf("public-query rejection became parser-repair authority: %#v", parserRepair)
			}
			return nil
		},
	}
	repairModelResolutions := 0
	_, err = generateDirectCodingTypeScriptBlockWithRuntime(
		runtime, "initial", func() (string, string, error) {
			repairModelResolutions++
			return "guidance", "correction", nil
		}, directCodingTypeScriptRepairEvents{}, job,
	)
	if err == nil || !strings.Contains(err.Error(), "deterministic public-surface grounding") {
		t.Fatalf("terminal public-query rejection=%v", err)
	}
	if executions != 1 || finalizations != 1 || repairModelResolutions != 0 {
		t.Fatalf(
			"public-query rejection escaped terminal boundary: executions=%d finalizations=%d repair_routes=%d",
			executions, finalizations, repairModelResolutions,
		)
	}
}
