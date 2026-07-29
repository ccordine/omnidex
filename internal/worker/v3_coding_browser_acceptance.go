package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericBrowserAcceptanceDocuments(
	specification assemblyline.ApplicationSpecification,
) []assemblyline.TypeScriptDocument {
	documents := make([]assemblyline.TypeScriptDocument, 0, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		functionName := fmt.Sprintf("Feature%03d", sequence)
		verifyName := fmt.Sprintf("VerifyFeature%03d", sequence)
		featureID := fmt.Sprintf("feature.%03d", sequence)
		verificationID := fmt.Sprintf("acceptance.%03d", sequence)
		documents = append(documents, assemblyline.TypeScriptDocument{
			ID:   fmt.Sprintf("acceptance_%03d", sequence),
			Path: fmt.Sprintf("src/features/%s.test.tsx", functionName),
			Header: fmt.Sprintf(`import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createApplicationRuntime, createFeatureRuntime } from '../runtime';
import { %s } from './%s';`, functionName, functionName),
			Blocks: []assemblyline.TypeScriptBlock{
				{
					ID:        verificationID,
					Signature: fmt.Sprintf("async function %s(): Promise<void>", verifyName),
					Contract: genericBrowserAcceptanceContract(
						requirement, specification.ProductQuote, functionName, genericBrowserCapabilityID(sequence),
					),
					API:          fmt.Sprintf("async function %s(): Promise<void>", verifyName),
					DependsOn:    []string{"runtime.factory", featureID},
					Capabilities: []string{"runtime.factory", featureID},
					Globals: []string{
						"React", "fireEvent", "render", "screen", "waitFor", "expect",
						"createApplicationRuntime", "createFeatureRuntime", functionName,
					},
					Policy: assemblyline.TypeScriptFunctionPolicy{
						RequiredCalls: []assemblyline.TypeScriptCallRequirement{
							{Callees: []string{"render"}}, {Callees: []string{"expect"}},
						},
						ForbiddenIdentifiers: append([]string(nil), acceptanceForbiddenHostAPIs...),
					},
					FailureTarget: featureID,
				},
				{
					ID: fmt.Sprintf("acceptance.register.%03d", sequence),
					Static: fmt.Sprintf(
						"it(%s, %s);", strconv.Quote("delivers "+requirement.SourceQuote), verifyName,
					),
					API:       "registered independent acceptance for " + requirement.ID,
					DependsOn: []string{verificationID},
				},
			},
		})
	}
	return documents
}

func genericBrowserAcceptanceContract(
	requirement assemblyline.Requirement,
	productQuote string,
	functionName string,
	capabilityID string,
) string {
	return strings.Join([]string{
		"Independently verify observable user behavior for this exact accepted feature: " + requirement.SourceQuote,
		"Product: " + productQuote,
		fmt.Sprintf(
			"Render <%s runtime={createFeatureRuntime(createApplicationRuntime(), %s)} /> exactly once.",
			functionName, strconv.Quote(capabilityID),
		),
		"Find controls by accessible role, label, or visible text. Perform a realistic interaction when the feature exposes one, then make at least one specific expect assertion about its observable UI result. Use waitFor for asynchronous behavior. Do not inspect source, internal state, class names, data attributes, or implementation details. Do not merely assert that rendering did not throw or that the component exists.",
	}, "\n")
}
