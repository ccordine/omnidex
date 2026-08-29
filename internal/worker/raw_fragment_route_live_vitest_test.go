package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const liveGuidedTSXBehaviorTimeout = 2 * time.Minute

func requireLiveGuidedTSXBehavior(
	t *testing.T,
	fixtureName, available, functionName, current, candidate, testBody string,
) {
	t.Helper()
	for _, required := range []struct {
		label string
		value string
	}{
		{label: "fixture name", value: fixtureName},
		{label: "function name", value: functionName},
		{label: "current declaration", value: current},
		{label: "candidate declaration", value: candidate},
		{label: "test body", value: testBody},
	} {
		if strings.TrimSpace(required.value) == "" {
			t.Fatalf("live guided TSX behavior requires a non-empty %s", required.label)
		}
	}
	if current == candidate {
		t.Fatal("live guided TSX behavior requires different current and candidate declarations")
	}

	currentOutput, currentErr := runLiveGuidedTSXBehaviorVariant(
		t, fixtureName, available, functionName, current, testBody,
	)
	if currentErr == nil {
		t.Fatalf("current TSX declaration unexpectedly passed real Testing Library behavior:\n%s", currentOutput)
	}
	if !strings.Contains(currentOutput, "TestingLibraryElementError") {
		t.Fatalf(
			"current TSX declaration failed without TestingLibraryElementError: %v\n%s",
			currentErr, currentOutput,
		)
	}

	candidateOutput, candidateErr := runLiveGuidedTSXBehaviorVariant(
		t, fixtureName, available, functionName, candidate, testBody,
	)
	if candidateErr != nil {
		t.Fatalf("candidate TSX declaration failed real Testing Library behavior: %v\n%s", candidateErr, candidateOutput)
	}
}

func runLiveGuidedTSXBehaviorVariant(
	t *testing.T,
	fixtureName, available, functionName, declaration, testBody string,
) (string, error) {
	t.Helper()
	nodeModules, err := filepath.Abs(filepath.Join("..", "api", "web", "node_modules"))
	if err != nil {
		t.Fatalf("resolve pinned web node_modules: %v", err)
	}
	for _, required := range []string{
		".bin/vitest",
		"react/package.json",
		"jsdom/package.json",
		"@testing-library/react/package.json",
		"@testing-library/jest-dom/package.json",
	} {
		if _, err := os.Stat(filepath.Join(nodeModules, required)); err != nil {
			t.Fatalf("live guided TSX behavior requires pinned web dependency %s: %v", required, err)
		}
	}

	root := t.TempDir()
	if err := os.Symlink(nodeModules, filepath.Join(root, "node_modules")); err != nil {
		t.Fatalf("link pinned web node_modules: %v", err)
	}
	config := `import { defineConfig } from 'vitest/config';

export default defineConfig({
  cacheDir: '.vite-cache',
  test: {
    environment: 'jsdom',
    include: ['behavior.test.tsx'],
  },
});
`
	if err := os.WriteFile(filepath.Join(root, "vitest.config.mjs"), []byte(config), 0o600); err != nil {
		t.Fatalf("write live guided TSX Vitest config: %v", err)
	}
	testName, err := json.Marshal(fixtureName + ": " + functionName)
	if err != nil {
		t.Fatalf("encode live guided TSX fixture name: %v", err)
	}
	testSource := fmt.Sprintf(`import '@testing-library/jest-dom/vitest';
import React, { type ReactElement } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

afterEach(cleanup);

%s

%s

it(%s, async () => {
%s
});
`, available, declaration, testName, testBody)
	if err := os.WriteFile(filepath.Join(root, "behavior.test.tsx"), []byte(testSource), 0o600); err != nil {
		t.Fatalf("write live guided TSX behavior test: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), liveGuidedTSXBehaviorTimeout)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		filepath.Join(nodeModules, ".bin", "vitest"),
		"run", "--config", "vitest.config.mjs", "behavior.test.tsx", "--reporter=verbose",
	)
	command.Dir = root
	command.Env = append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0")
	output, commandErr := command.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("Vitest timed out after %s: %w", liveGuidedTSXBehaviorTimeout, ctx.Err())
	}
	return string(output), commandErr
}

func TestLiveGuidedTSXBehaviorHarness(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to exercise the pinned Vitest behavior harness")
	}
	requireLiveGuidedTSXBehavior(
		t,
		"behavior harness",
		"interface BehaviorFixtureProps { readonly onActivate: () => void }",
		"BehaviorFixture",
		`function BehaviorFixture({ onActivate }: BehaviorFixtureProps): ReactElement {
  return <button onClick={onActivate}>idle</button>;
}`,
		`function BehaviorFixture({ onActivate }: BehaviorFixtureProps): ReactElement {
  return <button aria-label="activate" onClick={onActivate}>idle</button>;
}`,
		`  const onActivate = vi.fn();
  render(<BehaviorFixture onActivate={onActivate} />);
  fireEvent.click(screen.getByRole('button', { name: 'activate' }));
  expect(onActivate).toHaveBeenCalledTimes(1);`,
	)
}
