package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceStrictQueryForms(t *testing.T) {
	source := `async function VerifyInventory(): Promise<void> {
  fireEvent.change(screen.getAllByRole('textbox')[0], { target: { value: 'AX-7' } });
  fireEvent.input((await screen.findAllByRole('textbox'))[1], { target: { value: 'L2' } });
  fireEvent.click(await screen.findByRole('button', { name: 'Apply adjustment' }));
  expect(await screen.findByRole('textbox', { name: 'Stock code' })).toHaveValue('AX-7');
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, browserAcceptanceInventorySurface(), browserAcceptanceNoDerivedResult,
	); err != nil {
		t.Fatalf("strict query forms were rejected: %v", err)
	}
}

func TestBrowserAcceptanceRejectsScreenQueryEscapes(t *testing.T) {
	tests := map[string]struct {
		query string
		want  string
	}{
		"test id": {
			query: `screen.getByTestId('inventory')`, want: "unsupported by the grounded allowlist",
		},
		"label text": {
			query: `screen.getByLabelText('Stock code')`, want: "unsupported by the grounded allowlist",
		},
		"placeholder": {
			query: `screen.getByPlaceholderText('Stock')`, want: "unsupported by the grounded allowlist",
		},
		"debug": {
			query: `screen.debug()`, want: "unsupported by the grounded allowlist",
		},
		"unknown": {
			query: `screen.inspect('Stock')`, want: "screen query inspect is unsupported",
		},
		"nonthrowing text": {
			query: `screen.queryByText('Updated')`, want: "unsupported by the grounded allowlist",
		},
		"all text": {
			query: `screen.getAllByText('Updated')`, want: "unsupported by the grounded allowlist",
		},
		"indirect outcome query": {
			query: `panel.getByText('Updated')`, want: "must be a direct screen query",
		},
		"regex outcome": {
			query: `screen.getByText(/updated/i)`, want: "one non-empty exact text literal",
		},
		"outcome options": {
			query: `screen.getByText('Updated', { exact: false })`, want: "one exact text literal",
		},
		"unawaited outcome": {
			query: `screen.findByText('Updated')`, want: "must be explicitly awaited",
		},
		"unawaited singular role": {
			query: `screen.findByRole('button')`, want: "must be explicitly awaited",
		},
		"plural missing index": {
			query: `screen.getAllByRole('textbox')`, want: "requires an exact literal index",
		},
		"plural named": {
			query: `screen.getAllByRole('textbox', { name: 'Stock code' })[0]`, want: "forbids name filters",
		},
		"find all not awaited": {
			query: `screen.findAllByRole('textbox')[0]`, want: "awaited before indexing",
		},
		"find all awaited after indexing": {
			query: `await screen.findAllByRole('textbox')[0]`, want: "awaited before indexing",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			source := "async function Verify(): Promise<void> { expect(" + test.query + ").not.toBeNull(); }"
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, browserAcceptanceInventorySurface(), browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestBrowserAcceptanceRequiresExecutedQueryAssertion(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"no assertion": {
			source: `function Verify(): void { screen.getByText('Updated'); }`,
			want:   "must be consumed directly",
		},
		"expect without query": {
			source: `function Verify(): void { screen.getByText('Updated'); expect(true).toBe(true); }`,
			want:   "must be consumed directly",
		},
		"expect without matcher": {
			source: `function Verify(): void { expect(screen.getByText('Updated')); }`,
			want:   "must invoke an assertion matcher",
		},
		"dead arrow closure": {
			source: `function Verify(): void { const later = () => expect(screen.getByText('Updated')).toBeInTheDocument(); }`,
			want:   "nested or dead closures",
		},
		"nested function": {
			source: `function Verify(): void { function Later(): void { expect(screen.getByText('Updated')).toBeInTheDocument(); } }`,
			want:   "nested or dead closures",
		},
		"if branch": {
			source: `function Verify(): void { if (ready) { expect(screen.getByText('Updated')).toBeInTheDocument(); } }`,
			want:   "conditional or control-flow branches",
		},
		"logical branch": {
			source: `function Verify(): void { ready && expect(screen.getByText('Updated')).toBeInTheDocument(); }`,
			want:   "conditional expressions",
		},
		"loop branch": {
			source: `function Verify(): void { while (ready) { expect(screen.getByText('Updated')).toBeInTheDocument(); } }`,
			want:   "conditional or control-flow branches",
		},
		"waitFor conditional": {
			source: `async function Verify(): Promise<void> { await waitFor(() => { if (ready) expect(screen.getByText('Updated')).toBeInTheDocument(); }); }`,
			want:   "conditional or control-flow branches",
		},
		"unawaited waitFor": {
			source: `async function Verify(): Promise<void> { waitFor(() => expect(screen.getByText('Updated')).toBeInTheDocument()); }`,
			want:   "waitFor must be explicitly awaited",
		},
		"unreachable after return": {
			source: `function Verify(): void { return; expect(screen.getByText('Updated')).toBeInTheDocument(); }`,
			want:   "one flat sequence of expression statements",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				test.source, true, browserAcceptanceInventorySurface(), browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestBrowserAcceptanceRejectsAuthorityShadowing(t *testing.T) {
	tests := map[string]string{
		"screen parameter":   `function Verify(screen: unknown): void { expect(true).toBe(true); }`,
		"expect variable":    `function Verify(): void { const expect = () => undefined; }`,
		"fireEvent pattern":  `function Verify(): void { const { click: fireEvent } = source; }`,
		"waitFor parameter":  `function Verify(): void { function Later(waitFor: unknown): void {} }`,
		"screen declaration": `function screen(): void {}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, browserAcceptanceInventorySurface(), browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), "shadows permitted direct symbol") {
				t.Fatalf("authority shadow was accepted: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceExactStringsDecodeDisplayEscapes(t *testing.T) {
	surface := directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
		{
			Role: "button", RoleOrdinal: 1, RoleCount: 1,
			AccessibleName: `Say "hello" at C:\Temp's desk`, ValueKind: "action",
		},
	}}
	valid := `function Verify(): void {
  expect(screen.getByRole('button', { name: 'Say "hello" at C:\\Temp\'s desk' })).toBeInTheDocument();
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		valid, true, surface, browserAcceptanceNoDerivedResult,
	); err != nil {
		t.Fatalf("escaped exact public name was rejected: %v", err)
	}
	for name, literal := range map[string]string{
		"unknown escape": `'bad\qvalue'`,
		"control escape": `'bad\nvalue'`,
	} {
		t.Run(name, func(t *testing.T) {
			source := "function Verify(): void { expect(screen.getByText(" + literal + ")).toBeInTheDocument(); }"
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, surface, browserAcceptanceNoDerivedResult,
			); err == nil {
				t.Fatalf("invalid display escape %s was accepted", literal)
			}
		})
	}
}
