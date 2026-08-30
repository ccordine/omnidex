package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceNamedStatusOutputExactMatcher(t *testing.T) {
	surface := browserAcceptanceOutputSurface()
	for name, source := range map[string]string{
		"synchronous": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);
}`,
		"asynchronous": `async function Verify(): Promise<void> {
  expect(await screen.findByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);
}`,
		"escaped literal": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^tag \$alpha\.v1$/);
}`,
		"escaped slash": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^alpha\/beta$/);
}`,
		"escaped anchors": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^\^alpha\$$/);
}`,
		"escaped backslash": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^C:\\Cache$/);
}`,
		"unicode literal": `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^CAFÉ$/);
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
		"substring string":      `toHaveTextContent('ALPHA')`,
		"unanchored":            `toHaveTextContent(/ALPHA/)`,
		"leading substring":     `toHaveTextContent(/^ALPHA/)`,
		"trailing substring":    `toHaveTextContent(/ALPHA$/)`,
		"wildcard":              `toHaveTextContent(/^.*ALPHA.*$/)`,
		"single wildcard":       `toHaveTextContent(/^ALP.A$/)`,
		"alternation":           `toHaveTextContent(/^(ALPHA|ALPHABET)$/)`,
		"character class":       `toHaveTextContent(/^[AB]LPHA$/)`,
		"flag":                  `toHaveTextContent(/^ALPHA$/i)`,
		"dynamic regex":         `toHaveTextContent(new RegExp('^ALPHA$'))`,
		"negated exact matcher": `not.toHaveTextContent(/^ALPHA$/)`,
		"hex escape":            `toHaveTextContent(/^\x41LPHA$/)`,
		"unicode escape":        `toHaveTextContent(/^\u0041LPHA$/)`,
		"identity escape":       `toHaveTextContent(/^\q$/)`,
	}
	for name, matcher := range matchers {
		t.Run(name, func(t *testing.T) {
			source := `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).` + matcher + `;
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

func TestBrowserAcceptanceAnchoredLiteralSeparatesPrefixFromLongerValue(t *testing.T) {
	surface := browserAcceptanceOutputSurface()
	exact := `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		exact, true, surface, browserAcceptanceExplicitResult,
	); err != nil {
		t.Fatalf("anchored expected label was rejected: %v", err)
	}
	substring := `function Verify(): void {
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/ALPHA/);
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		substring, true, surface, browserAcceptanceExplicitResult,
	); err == nil {
		t.Fatal("substring expected ALPHA could match actual ALPHABET")
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
			body:    `expect(screen.getByRole('status', { name: 'Unrelated result' })).toHaveTextContent(/^ALPHA$/);`,
			want:    `no status output with accessible name "Unrelated result"`,
		},
		"missing output name": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.getByRole('status')).toHaveTextContent(/^ALPHA$/);`,
			want:    "requires one exact receipt accessible name",
		},
		"plural output query": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.getAllByRole('status')[0]).toHaveTextContent(/^ALPHA$/);`,
			want:    "must be singular",
		},
		"unawaited output query": {
			surface: browserAcceptanceOutputSurface(),
			body:    `expect(screen.findByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);`,
			want:    "must be explicitly awaited",
		},
		"no dynamic output": {
			surface: browserAcceptanceGrammarSurface(),
			body:    `expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);`,
			want:    `no status output with accessible name "Normalized label"`,
		},
		"empty receipt output": {
			surface: directCodingBrowserPublicInteractionSurface{
				Outputs: []directCodingBrowserPublicOutput{{}},
			},
			body: `expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);`,
			want: "requires an accessible name",
		},
		"status fireEvent": {
			surface: browserAcceptanceOutputSurface(),
			body: `fireEvent.click(screen.getByRole('status', { name: 'Normalized label' }));
  expect(screen.getByRole('status', { name: 'Normalized label' })).toHaveTextContent(/^ALPHA$/);`,
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
		{AccessibleName: "Normalized label"},
	}
	return surface
}
