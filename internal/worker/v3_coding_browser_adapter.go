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
) (string, assemblyline.TypeScriptBlueprint, []directCodingFileTask, error) {
	if err := specification.Validate(); err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, err
	}
	if specification.Surface != assemblyline.ApplicationSurfaceBrowser {
		return "", assemblyline.TypeScriptBlueprint{}, nil, fmt.Errorf(
			"generic TypeScript browser adapter does not support surface %s",
			specification.Surface,
		)
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(applicationWorkloadInput(specification), workload)
	if err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, err
	}
	documents := []assemblyline.TypeScriptDocument{genericBrowserRuntimeDocument(specification.Requirements)}
	featureDocuments, err := genericBrowserFeatureDocuments(specification, skills, contexts, capabilities)
	if err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, err
	}
	documents = append(documents, featureDocuments...)
	acceptanceDocuments, err := genericBrowserAcceptanceDocuments(specification, contexts, capabilities)
	if err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, err
	}
	documents = append(documents, acceptanceDocuments...)
	documents = append(documents, genericBrowserAppDocument(specification))
	documents = append(documents, genericBrowserSmokeTestDocument(specification))
	documents = append(documents, genericBrowserRuntimeTestDocument(specification.Requirements))
	blueprint := assemblyline.TypeScriptBlueprint{Documents: documents}
	if err := blueprint.Validate(); err != nil {
		return "", assemblyline.TypeScriptBlueprint{}, nil, fmt.Errorf("validate generic browser blueprint: %w", err)
	}
	staticFiles := typeScriptBrowserStaticFiles(
		packageName,
		specification.ProductQuote,
		genericBrowserStylesSource(),
	)
	return genericTypeScriptBrowserAdapter, blueprint, staticFiles, nil
}

func genericBrowserFeatureDocuments(
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
) ([]assemblyline.TypeScriptDocument, error) {
	documents := make([]assemblyline.TypeScriptDocument, 0, len(specification.Requirements))
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
		behavior, err := compileDirectCodingApplicationTaskBehavior(taskContext, dependencies)
		if err != nil {
			return nil, err
		}
		documents = append(documents, assemblyline.TypeScriptDocument{
			ID:   fmt.Sprintf("feature_%03d", sequence),
			Path: fmt.Sprintf("src/features/%s.tsx", functionName),
			Header: `import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactElement } from 'react';
import { FeatureBoundary } from '../runtime';
import type { CapabilitySnapshot, FeatureActions, FeatureProps, FeatureState, FeatureViewProps, SharedValue } from '../runtime';`,
			Blocks: []assemblyline.TypeScriptBlock{
				{
					ID: contextID, Static: genericBrowserFeatureProjectionSource(viewPropsName, dependencies),
					API:       genericBrowserFeatureProjectionAPI(viewPropsName, dependencies),
					DependsOn: []string{"runtime.api"},
				},
				{
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
				},
				{
					ID: wrapperID, Static: fmt.Sprintf(
						"export function %s({ runtime }: FeatureProps): ReactElement { return <FeatureBoundary runtime={runtime} view={%s} />; }",
						functionName, viewName,
					),
					API:       fmt.Sprintf("function %s({ runtime }: FeatureProps): ReactElement", functionName),
					DependsOn: []string{"runtime.api", blockID},
				},
			},
		})
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
		"The task-neutral code-owned boundary supplies state, read-only capability snapshots, mutations, live working status, and visible errors.",
		"All shared state changes go through actions inside user interaction handlers. Use state for this feature and capabilities only when another feature materially affects this view.",
		"React hooks and standard browser APIs are available when the requested behavior needs them. Implement workload-specific behavior here; do not assume Omnidex provides a domain service.",
	)
	parts = append(parts, "Use only listed capability identifiers; do not invent identifiers.")
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

func genericBrowserFeaturePolicy() assemblyline.TypeScriptFunctionPolicy {
	return assemblyline.TypeScriptFunctionPolicy{}
}
