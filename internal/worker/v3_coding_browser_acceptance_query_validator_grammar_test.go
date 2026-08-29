package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceExecutionGrammarAcceptsOnlyRegisteredDirectForms(t *testing.T) {
	source := `async function Verify(): Promise<void> {
  fireEvent.change(screen.getByRole('textbox', { name: 'Display name' }), { target: { value: 'Ada' } });
  expect(screen.getByRole('textbox', { name: 'Display name' })).toHaveValue('Ada');
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'Display name' })).toHaveValue('Ada'));
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, browserAcceptanceGrammarSurface(), browserAcceptanceNoDerivedResult,
	); err != nil {
		t.Fatalf("direct execution grammar was rejected: %v", err)
	}
}

func TestBrowserAcceptanceExecutionGrammarRejectsRootSideEffects(t *testing.T) {
	tests := map[string]string{
		"arbitrary call": `Object.assign({}, {});
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();`,
		"reflection call": `({}).constructor.constructor('return globalThis')();
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();`,
		"nested fire event": `Boolean(fireEvent.click(screen.getByRole('button', { name: 'Submit' })));
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();`,
		"nested assertion": `Boolean(expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument());`,
		"sequence":         `(expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument(), Object.freeze({}));`,
		"awaited arbitrary call": `await Promise.resolve();
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := `async function Verify(): Promise<void> { ` + body + ` }`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, browserAcceptanceGrammarSurface(), browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), "execution expression must be") {
				t.Fatalf("root side effect was accepted or returned the wrong error: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceExecutionGrammarRejectsWaitForSideEffects(t *testing.T) {
	tests := map[string]string{
		"arbitrary callback call": `await waitFor(() => {
  Promise.resolve();
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();
});`,
		"callback fire event": `await waitFor(() => {
  fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();
});`,
		"nested callback assertion": `await waitFor(() => Boolean(
  expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument()
));`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := `async function Verify(): Promise<void> { ` + body + ` }`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, browserAcceptanceGrammarSurface(), browserAcceptanceNoDerivedResult,
			)
			if err == nil {
				t.Fatal("waitFor callback side effect was accepted")
			}
			if name != "callback fire event" &&
				!strings.Contains(err.Error(), "execution expression must be") {
				t.Fatalf("waitFor side effect returned the wrong error: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceExecutionGrammarRejectsMatcherSideEffects(t *testing.T) {
	tests := map[string]string{
		"dynamic value argument": `expect(screen.getByRole('textbox', { name: 'Display name' })).toHaveValue(String('Ada'));`,
		"arbitrary matcher":      `expect(screen.getByRole('button', { name: 'Submit' })).toBe(Object.assign({}, {}));`,
		"presence argument":      `expect(screen.getByText('Saved')).toBeInTheDocument(recordObservation());`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := `function Verify(): void { ` + body + ` }`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, browserAcceptanceGrammarSurface(), browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), "unsupported or has non-static") {
				t.Fatalf("matcher side effect was accepted or returned the wrong error: %v", err)
			}
		})
	}
}

func browserAcceptanceGrammarSurface() directCodingBrowserPublicInteractionSurface {
	return directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
		{
			Role: "textbox", RoleOrdinal: 1, RoleCount: 1,
			AccessibleName: "Display name", ValueKind: "text",
		},
		{
			Role: "button", RoleOrdinal: 1, RoleCount: 1,
			AccessibleName: "Submit", ValueKind: "action",
		},
	}}
}
