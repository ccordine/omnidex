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
) ([]assemblyline.TypeScriptDocument, error) {
	documents := make([]assemblyline.TypeScriptDocument, 0, len(specification.Requirements))
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
		behavior, err := compileDirectCodingApplicationTaskBehavior(
			taskContext, capabilities[requirement.ID],
		)
		if err != nil {
			return nil, err
		}
		documents = append(documents, assemblyline.TypeScriptDocument{
			ID:   fmt.Sprintf("acceptance_%03d", sequence),
			Path: fmt.Sprintf("src/features/%s.test.tsx", functionName),
			Header: fmt.Sprintf(`import '@testing-library/jest-dom/vitest';
import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createApplicationRuntime, createFeatureRuntime } from '../runtime';
import { %s } from './%s';`, functionName, functionName),
			Blocks: []assemblyline.TypeScriptBlock{
				{
					ID:        verificationID,
					Signature: fmt.Sprintf("async function %s(): Promise<void>", verifyName),
					Contract:  genericBrowserAcceptanceContract(behavior),
					API:       fmt.Sprintf("async function %s(): Promise<void>", verifyName),
					Globals:   []string{"fireEvent", "screen", "waitFor", "expect"},
					Policy: assemblyline.TypeScriptFunctionPolicy{
						RequiredCalls: []assemblyline.TypeScriptCallRequirement{
							{Callees: []string{"expect"}},
						},
						ForbiddenIdentifiers: append(
							append([]string(nil), acceptanceForbiddenHostAPIs...),
							"render", "createApplicationRuntime", "createFeatureRuntime", functionName,
						),
					},
				},
				{
					ID: harnessID,
					Static: genericBrowserAcceptanceHarnessSource(
						harnessName, verifyName, functionName, genericBrowserCapabilityID(sequence),
					),
					API: fmt.Sprintf("async function %s(): Promise<void>", harnessName),
					DependsOn: []string{
						"runtime.factory", wrapperID, verificationID,
					},
				},
				{
					ID: fmt.Sprintf("acceptance.register.%03d", sequence),
					Static: fmt.Sprintf(
						"it(%s, %s);", strconv.Quote("delivers "+requirement.SourceQuote), harnessName,
					),
					API:       "registered independent acceptance for " + requirement.ID,
					DependsOn: []string{harnessID},
				},
			},
		})
	}
	return documents, nil
}

func genericBrowserAcceptanceContract(
	behavior string,
) string {
	return strings.Join([]string{
		behavior,
		"The code-owned harness renders the public component before invoking this function. Assert every observable acceptance criterion using direct screen queries only as standalone throwing observations, expect subjects, or fireEvent targets. Use only static arguments and event payloads. Await findBy, findAllBy, and waitFor; a waitFor callback may contain only those same direct forms. Do not create aliases, local proof values, UI, arbitrary calls, assignments, or control flow. Do not inspect source or internal state, use class names or data attributes, or merely assert that the component exists.",
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
