package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceNamedStatusOutputExactMatcher(t *testing.T) {
	surface := browserAcceptanceOutputSurface()
	for name, source := range map[string]string{
		"synchronous": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);
}`,
		"asynchronous": `async function Verify(): Promise<void> {
  expect(await screen.findByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);
}`,
		"escaped literal": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^USD \$42\.00$/);
}`,
		"escaped slash": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42\/100$/);
}`,
		"escaped anchors": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^\^42\$$/);
}`,
		"escaped backslash": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^C:\\Temp$/);
}`,
		"unicode literal": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^café$/);
}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, surface, browserAcceptanceExplicitResult,
			); err != nil {
				t.Fatalf("exact named output was rejected: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceNamedStatusOutputRejectsSubstringAuthority(t *testing.T) {
	surface := browserAcceptanceOutputSurface()
	matchers := map[string]string{
		"empty expected output": `toHaveTextContent(/^$/)`,
		"substring string":      `toHaveTextContent('42')`,
		"unanchored 42":         `toHaveTextContent(/42/)`,
		"leading substring":     `toHaveTextContent(/^42/)`,
		"trailing substring":    `toHaveTextContent(/42$/)`,
		"wildcard":              `toHaveTextContent(/^.*42.*$/)`,
		"single wildcard":       `toHaveTextContent(/^4.2$/)`,
		"alternation":           `toHaveTextContent(/^(42|142)$/)`,
		"character class":       `toHaveTextContent(/^[14]42$/)`,
		"flag":                  `toHaveTextContent(/^42$/i)`,
		"dynamic regex":         `toHaveTextContent(new RegExp('^42$'))`,
		"negated exact matcher": `not.toHaveTextContent(/^42$/)`,
		"hex escape":            `toHaveTextContent(/^\x34\x32$/)`,
		"unicode escape":        `toHaveTextContent(/^\u0034\u0032$/)`,
		"identity escape":       `toHaveTextContent(/^\q$/)`,
	}
	for name, matcher := range matchers {
		t.Run(name, func(t *testing.T) {
			source := `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).` + matcher + `;
}`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, surface, browserAcceptanceExplicitResult,
			)
			if err == nil || !strings.Contains(err.Error(), "unsupported or has non-static") {
				t.Fatalf("substring or pattern authority was accepted: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceAnchoredLiteralSeparates42From142(t *testing.T) {
	surface := browserAcceptanceOutputSurface()
	exact := `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		exact, true, surface, browserAcceptanceExplicitResult,
	); err != nil {
		t.Fatalf("anchored expected 42 was rejected: %v", err)
	}
	substring := `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/42/);
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		substring, true, surface, browserAcceptanceExplicitResult,
	); err == nil {
		t.Fatal("substring expected 42 could match actual 142")
	}
}

func TestBrowserAcceptanceNamedStatusOutputRejectsUngroundedQueriesAndEvents(t *testing.T) {
	tests := map[string]struct {
		surface directCodingBrowserPublicInteractionSurface
		body    string
		want    string
	}{
		"wrong output name": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.getByRole('status', { name: 'Unrelated result' })).toHaveTextContent(/^42$/);`,
			want:    `no status output with accessible name "Unrelated result"`,
		},
		"missing output name": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.getByRole('status')).toHaveTextContent(/^42$/);`,
			want:    "requires one exact receipt accessible name",
		},
		"plural output query": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.getAllByRole('status')[0]).toHaveTextContent(/^42$/);`,
			want:    "must be singular",
		},
		"unawaited output query": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.findByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);`,
			want:    "must be explicitly awaited",
		},
		"no dynamic output": {
			surface: browserAcceptanceGrammarSurface(),
			body:    `expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);`,
			want:    `no status output with accessible name "Calculation result"`,
		},
		"empty receipt output": {
			surface: directCodingBrowserPublicInteractionSurface{
				Outputs: []directCodingBrowserPublicOutput{{}},
			},
			body: `expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);`,
			want: "requires an accessible name",
		},
		"status fireEvent": {
			surface: browserAcceptanceOutputSurface(),
			body: `fireEvent.click(screen.getByRole('status', { name: 'Calculation result' }));
  expect(screen.getByRole('status', { name: 'Calculation result' })).toHaveTextContent(/^42$/);`,
			want: "target is not one exact grounded role query",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			source := `async function Verify(): Promise<void> { ` + test.body + ` }`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, test.surface, browserAcceptanceExplicitResult,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func browserAcceptanceOutputSurface() directCodingBrowserPublicInteractionSurface {
	surface := browserAcceptanceGrammarSurface()
	surface.Outputs = []directCodingBrowserPublicOutput{
		{AccessibleName: "Calculation result"},
	}
	return surface
}
