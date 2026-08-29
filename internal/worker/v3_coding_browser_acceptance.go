package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericBrowserAcceptanceDocuments(
	specification assemblyline.ApplicationSpecification,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	documents := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	documentByPath := make(map[string]int, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		functionName := fmt.Sprintf("Feature%03d", sequence)
		verifyName := fmt.Sprintf("VerifyFeature%03d", sequence)
		harnessName := fmt.Sprintf("RunFeature%03dAcceptance", sequence)
		wrapperID := fmt.Sprintf("feature.wrapper.%03d", sequence)
		verificationID := fmt.Sprintf("acceptance.%03d", sequence)
		harnessID := fmt.Sprintf("acceptance.harness.%03d", sequence)
		taskContext, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("application workload omits requirement %s", requirement.ID)
		}
		files, err := directCodingTaskSinglePair(coverage, taskContext.Task.TaskID)
		if err != nil {
			return nil, err
		}
		behavior, err := compileDirectCodingApplicationTaskBehavior(
			taskContext, capabilities[requirement.ID],
		)
		if err != nil {
			return nil, err
		}
		documentIndex, exists := documentByPath[files.VerificationPath]
		if !exists {
			documentIndex = len(documents)
			documentByPath[files.VerificationPath] = documentIndex
			documents = append(documents, assemblyline.SourceDocument{
				ID:   fmt.Sprintf("acceptance_%03d", sequence),
				Path: files.VerificationPath,
				Preamble: genericBrowserAcceptancePreamble(
					typeScriptRelativeModule(files.VerificationPath, "src/runtime.tsx"),
				),
			})
		}
		taskID := taskContext.Task.TaskID
		documents[documentIndex].ScopedPreambles = append(
			documents[documentIndex].ScopedPreambles,
			assemblyline.SourcePreamble{
				TaskID: taskID,
				Source: fmt.Sprintf("import { %s } from '%s';",
					functionName, typeScriptRelativeModule(files.VerificationPath, files.ImplementationPath)),
			},
		)
		documents[documentIndex].Blocks = append(documents[documentIndex].Blocks,
			assemblyline.SourceBlock{
				ID:        verificationID,
				Signature: fmt.Sprintf("async function %s(): Promise<void>", verifyName),
				Contract:  genericBrowserAcceptanceContract(behavior),
				API:       fmt.Sprintf("async function %s(): Promise<void>", verifyName),
				DependsOn: []string{fmt.Sprintf("feature.%03d", sequence)},
				Globals:   []string{"fireEvent", "screen", "waitFor", "expect"},
				Policy: assemblyline.SourceFunctionPolicy{
					RequiredCalls: []assemblyline.SourceCallRequirement{
						{Callees: []string{"expect"}},
					},
					ForbiddenIdentifiers: append(
						append([]string(nil), acceptanceForbiddenHostAPIs...),
						"render", "createApplicationRuntime", "createFeatureRuntime", functionName,
					),
				},
				TaskID: taskID, Role: assemblyline.SourceBlockTaskVerification,
			},
			assemblyline.SourceBlock{
				ID: harnessID,
				Static: genericBrowserAcceptanceHarnessSource(
					harnessName, verifyName, functionName, genericApplicationCapabilityID(sequence),
				),
				API: fmt.Sprintf("async function %s(): Promise<void>", harnessName),
				DependsOn: []string{
					"runtime.factory", wrapperID, verificationID,
				},
				TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
			assemblyline.SourceBlock{
				ID: fmt.Sprintf("acceptance.register.%03d", sequence),
				Static: fmt.Sprintf(
					"it(%s, %s);", strconv.Quote("delivers "+requirement.SourceQuote), harnessName,
				),
				API:       "registered independent acceptance for " + requirement.ID,
				DependsOn: []string{harnessID},
				TaskID:    taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
		)
	}
	return documents, nil
}

func genericBrowserAcceptanceContract(
	behavior string,
) string {
	return strings.Join([]string{
		behavior,
		"The harness renders first. Use one flat sequence of direct fireEvent calls and screen-grounded expect assertions. No declarations, branches, loops, returns, helpers, nested calls, or side effects. An awaited waitFor callback may contain only direct expect assertions.",
		"Realize every explicit interaction condition before observing its outcome. Enter concrete static user values with fireEvent.change or fireEvent.input before activation; never assume preloaded values. Derive the unique expected result independently from the exact requirement relation and static payloads. An action name is only a selector claim, never a missing relation, expected value, or proof.",
		"Receipt accessible_name literals are the only named selectors. role_ordinal is one-based; all-query indexes are zero-based. Never invent names or use placeholder_hint.",
		"Use only screen.getByRole, awaited screen.findByRole, indexed screen.getAllByRole, awaited indexed screen.findAllByRole, screen.getByText, and awaited screen.findByText. Role status requires a singular query with its exact receipt name and is never a fireEvent target. Use only compatible direct fireEvent.click, fireEvent.change, and fireEvent.input.",
		"For a derived result, select its exact named status output and assert non-negated toHaveTextContent with one no-flags ^literal$ regex; escape regex metacharacters. Compute the literal independently: output names locate nodes but never supply results. getByText and findByText never prove output ownership.",
	}, "\n")
}

func genericBrowserAcceptanceHarnessSource(
	harnessName string,
	verifyName string,
	functionName string,
	capabilityID string,
) string {
	return fmt.Sprintf(`async function %s(): Promise<void> {
  render(<%s runtime={createFeatureRuntime(createApplicationRuntime(), %s)} />);
  await %s();
}`, harnessName, functionName, strconv.Quote(capabilityID), verifyName)
}
