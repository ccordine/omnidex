package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericTypeScriptBrowserAdapter = "typescript_browser_capabilities_v3"

func compileGenericTypeScriptBrowserBlueprint(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	profile directCodingProjectVersionProfile,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents := []assemblyline.SourceDocument{genericBrowserRuntimeDocument(specification.Requirements)}
	featureDocuments, err := genericBrowserFeatureDocuments(specification, contexts, capabilities, coverage)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents = append(documents, featureDocuments...)
	appDocument, err := genericBrowserAppDocument(specification, contexts, coverage)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents = append(documents, appDocument)
	documents = append(documents, genericBrowserEntrypointDocument())
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
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	documents := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	documentByPath := make(map[string]int, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		sequence := index + 1
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
		implementationPath, err := directCodingTaskSingleImplementationPath(
			coverage, taskContext.Task.TaskID,
		)
		if err != nil {
			return nil, err
		}
		behavior, err := compileDirectCodingApplicationTaskBehavior(taskContext, dependencies)
		if err != nil {
			return nil, err
		}
		documentIndex, exists := documentByPath[implementationPath]
		if !exists {
			documentIndex = len(documents)
			documentByPath[implementationPath] = documentIndex
			documents = append(documents, assemblyline.SourceDocument{
				ID:       fmt.Sprintf("feature_%03d", sequence),
				Path:     implementationPath,
				Preamble: genericBrowserFeaturePreamble(implementationPath, nil),
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
				Contract: genericBrowserFeatureContract(behavior),
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

func genericBrowserFeaturePreamble(
	implementationPath string,
	runtimeValueImports []string,
) string {
	values := append([]string{"FeatureBoundary"}, runtimeValueImports...)
	runtimeModule := typeScriptRelativeModule(implementationPath, "src/runtime.tsx")
	return fmt.Sprintf(`import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactElement } from 'react';
import { %s } from '%s';
import type { CapabilitySnapshot, FeatureActions, FeatureProps, FeatureState, FeatureViewProps, SharedValue } from '%s';`,
		strings.Join(values, ", "), runtimeModule, runtimeModule)
}

func genericBrowserFeatureContract(behavior string) string {
	return strings.Join([]string{
		behavior,
		"Return one React view implementing only this behavior; do not add imports, declarations, placeholders, or unrelated product rules.",
		"Use only the listed direct declarations and capability identifiers. Shared state changes use the supplied actions.",
	}, "\n")
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
