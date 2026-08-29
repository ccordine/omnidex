package worker

import "fmt"

const genericBrowserTestingLibraryRoleObservationSupport = `{
type OmnidexTestingLibraryRoleObservation = {
  schema: 'omnidex.testing-library-role-observation.v1';
  requested_role: string;
  visibility: 'accessible' | 'available';
  status: 'complete' | 'limit_exceeded' | 'capture_failed';
  element_count: number;
  names: string[];
};

type OmnidexTestingLibraryMissingRole = {
  requestedRole: string;
  visibility: 'accessible' | 'available';
};

const omnidexTestingLibraryRoleLimit = 64;
const omnidexTestingLibraryElementLimit = 100;
const omnidexTestingLibraryNameLimit = 256;
const omnidexTestingLibraryAccessibleRolePrefix = 'Unable to find an accessible element with the role "';
const omnidexTestingLibraryAvailableRolePrefix = 'Unable to find an element with the role "';

function omnidexTestingLibraryUTF8Bytes(value: string): number {
  let bytes = 0;
  for (const symbol of value) {
    const codePoint = symbol.codePointAt(0);
    if (codePoint === undefined) throw new Error('UTF-8 byte measurement received an empty symbol.');
    if (codePoint <= 0x7f) bytes += 1;
    else if (codePoint <= 0x7ff) bytes += 2;
    else if (codePoint <= 0xffff) bytes += 3;
    else bytes += 4;
  }
  return bytes;
}

function omnidexTestingLibraryBoundedRole(value: string): string {
  let bounded = '';
  let bytes = 0;
  for (const symbol of value) {
    const width = omnidexTestingLibraryUTF8Bytes(symbol);
    if (bytes + width > omnidexTestingLibraryRoleLimit) break;
    bounded += symbol;
    bytes += width;
  }
  return bounded;
}

function omnidexTestingLibraryMissingRole(message: string | null): OmnidexTestingLibraryMissingRole | null {
  if (message === null) return null;
  let prefix: string;
  let visibility: 'accessible' | 'available';
  if (message.startsWith(omnidexTestingLibraryAccessibleRolePrefix)) {
    prefix = omnidexTestingLibraryAccessibleRolePrefix;
    visibility = 'accessible';
  } else if (message.startsWith(omnidexTestingLibraryAvailableRolePrefix)) {
    prefix = omnidexTestingLibraryAvailableRolePrefix;
    visibility = 'available';
  } else {
    return null;
  }
  const closingQuote = message.indexOf('"', prefix.length);
  if (closingQuote < 0) return null;
  const requestedRole = message.slice(prefix.length, closingQuote);
  if (requestedRole.length === 0 || requestedRole.includes('\n') || requestedRole.includes('\r')) return null;
  const suffix = message.slice(closingQuote + 1);
  if (!suffix.startsWith('\n\n') && !suffix.startsWith(' and name ') && !suffix.startsWith(' and description ')) {
    return null;
  }
  return { requestedRole, visibility };
}

function omnidexTestingLibraryRoleObservation(
  message: string | null,
  container: Element,
): OmnidexTestingLibraryRoleObservation | null {
  const missingRole = omnidexTestingLibraryMissingRole(message);
  if (missingRole === null) return null;
  const requestedRole = omnidexTestingLibraryBoundedRole(missingRole.requestedRole);
  let elements: HTMLElement[];
  try {
    const roles = (getRoles as unknown as (
      root: HTMLElement,
      options: { hidden: boolean },
    ) => Record<string, HTMLElement[]>)(container as HTMLElement, {
      hidden: missingRole.visibility === 'available',
    });
    const candidate = Object.prototype.hasOwnProperty.call(roles, missingRole.requestedRole)
      ? roles[missingRole.requestedRole]
      : [];
    if (!Array.isArray(candidate)) {
      return {
        schema: 'omnidex.testing-library-role-observation.v1', requested_role: requestedRole,
        visibility: missingRole.visibility, status: 'capture_failed', element_count: 0, names: [],
      };
    }
    elements = candidate;
  } catch {
    return {
      schema: 'omnidex.testing-library-role-observation.v1', requested_role: requestedRole,
      visibility: missingRole.visibility, status: 'capture_failed', element_count: 0, names: [],
    };
  }
  const elementCount = elements.length;
  if (
    omnidexTestingLibraryUTF8Bytes(missingRole.requestedRole) > omnidexTestingLibraryRoleLimit
    || elementCount > omnidexTestingLibraryElementLimit
  ) {
    return {
      schema: 'omnidex.testing-library-role-observation.v1', requested_role: requestedRole,
      visibility: missingRole.visibility, status: 'limit_exceeded', element_count: elementCount, names: [],
    };
  }
  try {
    const names = elements.map((element) => computeAccessibleName(element));
    if (names.some((name) => omnidexTestingLibraryUTF8Bytes(name) > omnidexTestingLibraryNameLimit)) {
      return {
        schema: 'omnidex.testing-library-role-observation.v1', requested_role: requestedRole,
        visibility: missingRole.visibility, status: 'limit_exceeded', element_count: elementCount, names: [],
      };
    }
    return {
      schema: 'omnidex.testing-library-role-observation.v1', requested_role: requestedRole,
      visibility: missingRole.visibility, status: 'complete', element_count: elementCount, names,
    };
  } catch {
    return {
      schema: 'omnidex.testing-library-role-observation.v1', requested_role: requestedRole,
      visibility: missingRole.visibility, status: 'capture_failed', element_count: elementCount, names: [],
    };
  }
}

configure({
  getElementError: (message, container): Error => {
    const error = new Error(message === null ? '' : message);
    error.name = 'TestingLibraryElementError';
    const observation = omnidexTestingLibraryRoleObservation(message, container);
    if (observation !== null) {
      Object.defineProperty(error, 'omnidexTestingLibraryRoleObservation', {
        value: observation,
        enumerable: true,
        configurable: false,
        writable: false,
      });
    }
    return error;
  },
});
}`

func genericBrowserAcceptancePreamble(runtimeModule string) string {
	return fmt.Sprintf(`import '@testing-library/jest-dom/vitest';
// @ts-expect-error dom-accessibility-api 0.5.16 omits its declarations from package exports.
import { computeAccessibleName } from 'dom-accessibility-api';
import React from 'react';
import { configure, fireEvent, getRoles, render, screen, waitFor } from '@testing-library/react';
import { createApplicationRuntime, createFeatureRuntime } from '%s';

%s`, runtimeModule, genericBrowserTestingLibraryRoleObservationSupport)
}
