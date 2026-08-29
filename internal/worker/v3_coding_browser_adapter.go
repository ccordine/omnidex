package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericTypeScriptBrowserAdapter = "typescript_browser_capabilities_v3"

var acceptanceForbiddenHostAPIs = []string{
	"AudioContext", "Audio", "fetch", "XMLHttpRequest", "WebSocket", "EventSource",
	"Worker", "SharedWorker", "localStorage", "sessionStorage", "indexedDB",
	"window", "document", "navigator", "globalThis", "alert", "confirm", "prompt",
	"requestAnimationFrame", "cancelAnimationFrame",
}

func compileGenericTypeScriptBrowserBlueprint(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	if err := specification.Validate(); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if specification.Surface != assemblyline.ApplicationSurfaceBrowser {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"generic TypeScript browser adapter does not support surface %s",
			specification.Surface,
		)
	}
	profile, err := directCodingVersionProfileForTargetTree(target)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents := []assemblyline.SourceDocument{genericBrowserRuntimeDocument(specification.Requirements)}
	featureDocuments, err := genericBrowserFeatureDocuments(specification, skills, contexts, capabilities, coverage)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents = append(documents, featureDocuments...)
	acceptanceDocuments, err := genericBrowserAcceptanceDocuments(specification, contexts, capabilities, coverage)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents = append(documents, acceptanceDocuments...)
	appDocument, err := genericBrowserAppDocument(specification, contexts, coverage)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents = append(documents, appDocument)
	documents = append(documents, genericBrowserEntrypointDocument())
	documents = append(documents, genericBrowserSmokeTestDocument(specification))
	documents = append(documents, genericBrowserRuntimeTestDocument(specification.Requirements))
	blueprint := assemblyline.SourceBlueprint{Documents: documents}
	staticFiles, err := typeScriptBrowserStaticFiles(
		profile,
		packageName,
		specification.ProductQuote,
		genericBrowserStylesSource(),
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return blueprint, staticFiles, nil
}

func genericBrowserFeatureDocuments(
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	documents := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	documentByPath := make(map[string]int, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		skill, hasSkill := skills[requirement.ID]
		var activeSkill *directCodingSkillBinding
		if hasSkill {
			activeSkill = &skill
		}
		functionName := fmt.Sprintf("Feature%03d", sequence)
		viewName := functionName + "View"
		viewPropsName := functionName + "ViewProps"
		blockID := fmt.Sprintf("feature.%03d", sequence)
		contextID := fmt.Sprintf("feature.context.%03d", sequence)
		wrapperID := fmt.Sprintf("feature.wrapper.%03d", sequence)
		dependencies := capabilities[requirement.ID]
		taskContext, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("application workload omits requirement %s", requirement.ID)
		}
		files, err := directCodingTaskSinglePair(coverage, taskContext.Task.TaskID)
		if err != nil {
			return nil, err
		}
		behavior, err := compileDirectCodingApplicationTaskBehavior(taskContext, dependencies)
		if err != nil {
			return nil, err
		}
		documentIndex, exists := documentByPath[files.ImplementationPath]
		if !exists {
			documentIndex = len(documents)
			documentByPath[files.ImplementationPath] = documentIndex
			documents = append(documents, assemblyline.SourceDocument{
				ID:   fmt.Sprintf("feature_%03d", sequence),
				Path: files.ImplementationPath,
				Preamble: fmt.Sprintf(`import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactElement } from 'react';
import { FeatureBoundary } from '%s';
import type { CapabilitySnapshot, FeatureActions, FeatureProps, FeatureState, FeatureViewProps, SharedValue } from '%s';`,
					typeScriptRelativeModule(files.ImplementationPath, "src/runtime.tsx"),
					typeScriptRelativeModule(files.ImplementationPath, "src/runtime.tsx")),
			})
		}
		taskID := taskContext.Task.TaskID
		documents[documentIndex].Blocks = append(documents[documentIndex].Blocks,
			assemblyline.SourceBlock{
				ID: contextID, Static: genericBrowserFeatureProjectionSource(viewPropsName, dependencies),
				API:       genericBrowserFeatureProjectionAPI(viewPropsName, dependencies),
				DependsOn: []string{"runtime.api"},
				TaskID:    taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
			assemblyline.SourceBlock{
				ID: blockID,
				Signature: fmt.Sprintf(
					"function %s({ state, capabilities, actions }: %s): ReactElement", viewName, viewPropsName,
				),
				Contract: genericBrowserFeatureContract(behavior, activeSkill),
				API: fmt.Sprintf(
					"function %s({ state, capabilities, actions }: %s): ReactElement", viewName, viewPropsName,
				),
				DependsOn: []string{contextID}, Capabilities: []string{contextID},
				Globals: []string{
					"ReactElement", "useCallback", "useEffect", "useMemo", "useRef", "useState",
				},
				Policy: genericBrowserFeaturePolicy(),
				TaskID: taskID, Role: assemblyline.SourceBlockTaskImplementation,
			},
			assemblyline.SourceBlock{
				ID: wrapperID, Static: fmt.Sprintf(
					"export function %s({ runtime }: FeatureProps): ReactElement { return <FeatureBoundary runtime={runtime} view={%s} />; }",
					functionName, viewName,
				),
				API:       fmt.Sprintf("function %s({ runtime }: FeatureProps): ReactElement", functionName),
				DependsOn: []string{"runtime.api", blockID},
				TaskID:    taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
		)
	}
	return documents, nil
}

func genericBrowserFeatureContract(
	behavior string,
	skill *directCodingSkillBinding,
) string {
	parts := []string{behavior}
	if skill != nil {
		parts = append(parts, "Validated procedure: "+skill.Procedure)
	}
	parts = append(parts,
		"Return a complete accessible interactive React view. No placeholder, TODO, invented endpoint, import, or extra declaration.",
		"Tailwind CSS utility classes are available in className. Use complete static utility names; do not construct class names from fragments.",
		"State, read-only capability snapshots, mutations, live working status, and visible errors are available through the declared inputs.",
		"Route shared state changes through actions inside user interaction handlers. Read state for this behavior and capabilities only when another required behavior materially affects this view.",
		"React hooks and standard browser APIs are available when the required behavior needs them. Implement that behavior using only the declared inputs.",
	)
	parts = append(parts, "Every referenced capability identifier must be one of the listed capability identifiers.")
	return strings.Join(parts, "\n")
}

func genericBrowserFeatureProjectionSource(
	name string,
	dependencies []directCodingCapabilityBinding,
) string {
	keys := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		keys = append(keys, strconv.Quote(dependency.CapabilityID))
	}
	keyUnion := "never"
	if len(keys) > 0 {
		keyUnion = strings.Join(keys, " | ")
	}
	return fmt.Sprintf(
		"type %s = Omit<FeatureViewProps, 'capabilities'> & { readonly capabilities: Pick<CapabilitySnapshot, %s> };",
		name, keyUnion,
	)
}

func genericBrowserFeatureProjectionAPI(
	name string,
	dependencies []directCodingCapabilityBinding,
) string {
	capabilityFields := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		capabilityFields = append(capabilityFields, fmt.Sprintf(
			"readonly %s: SharedValue", dependency.CapabilityID,
		))
	}
	return strings.Join([]string{
		"type SharedValue = null | boolean | number | string | readonly SharedValue[] | { readonly [key: string]: SharedValue }",
		"type FeatureState = { readonly [key: string]: SharedValue }",
		genericBrowserFeatureActionsAPI(),
		fmt.Sprintf(
			"interface %s { readonly state: FeatureState; readonly capabilities: { %s }; readonly actions: FeatureActions }",
			name, strings.Join(capabilityFields, "; "),
		),
	}, "\n")
}

func genericBrowserFeaturePolicy() assemblyline.SourceFunctionPolicy {
	return assemblyline.SourceFunctionPolicy{}
}
