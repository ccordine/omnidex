package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceFireEventsRespectPublicValueKinds(t *testing.T) {
	source := `function VerifyControls(): void {
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'Ada' } });
  fireEvent.input(screen.getByRole('spinbutton'), { target: { value: '12.5' } });
  fireEvent.change(screen.getByRole('combobox'), { target: { value: 'priority' } });
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(screen.getByRole('radio'));
  fireEvent.click(screen.getByRole('button'));
  expect(screen.getByRole('textbox')).toHaveValue('Ada');
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, browserAcceptanceEventSurface(), browserAcceptanceNoDerivedResult,
	); err != nil {
		t.Fatalf("compatible public events were rejected: %v", err)
	}
}

func TestBrowserAcceptanceRejectsUngroundedOrIncompatibleEvents(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"click text": {
			body: `fireEvent.click(screen.getByRole('textbox'));`,
			want: "click is incompatible",
		},
		"click combobox": {
			body: `fireEvent.click(screen.getByRole('combobox'));`,
			want: "click is incompatible",
		},
		"change action": {
			body: `fireEvent.change(screen.getByRole('button'), { target: { value: 'go' } });`,
			want: "incompatible with public control value kind action",
		},
		"change boolean": {
			body: `fireEvent.change(screen.getByRole('checkbox'), { target: { value: 'true' } });`,
			want: "incompatible with public control value kind boolean",
		},
		"input selection": {
			body: `fireEvent.input(screen.getByRole('combobox'), { target: { value: 'priority' } });`,
			want: "selection controls require change",
		},
		"dynamic payload": {
			body: `fireEvent.change(screen.getByRole('textbox'), { target: { value: nextValue } });`,
			want: "target.value must be one static string or number literal",
		},
		"wrong payload shape": {
			body: `fireEvent.change(screen.getByRole('textbox'), { value: 'Ada' });`,
			want: "requires exact target property",
		},
		"extra payload field": {
			body: `fireEvent.change(screen.getByRole('textbox'), { target: { value: 'Ada' }, bubbles: true });`,
			want: "requires exact { target: ... } object shape",
		},
		"click options": {
			body: `fireEvent.click(screen.getByRole('button'), { bubbles: true });`,
			want: "requires exactly 1 arguments",
		},
		"outcome target": {
			body: `fireEvent.click(screen.getByText('Observed outcome'));`,
			want: "target is not one exact grounded role query",
		},
		"variable target": {
			body: `fireEvent.click(control);`,
			want: "target is not one exact grounded role query",
		},
		"post-indexed target": {
			body: `fireEvent.change(screen.getAllByRole('textbox')[0][0], { target: { value: 'Ada' } });`,
			want: "must be consumed directly",
		},
		"unsupported event": {
			body: `fireEvent.submit(screen.getByRole('button'));`,
			want: "fireEvent method submit is unsupported",
		},
		"aliased event": {
			body: `const trigger = fireEvent.click; trigger(screen.getByRole('button'));`,
			want: "fireEvent click must be called directly",
		},
		"invalid number": {
			body: `fireEvent.input(screen.getByRole('spinbutton'), { target: { value: 'many' } });`,
			want: "not a finite static number",
		},
		"numeric text payload": {
			body: `fireEvent.change(screen.getByRole('textbox'), { target: { value: 12 } });`,
			want: "requires a static string for a text control",
		},
		"dead event closure": {
			body: `const later = () => fireEvent.click(screen.getByRole('button'));`,
			want: "nested or dead closures",
		},
		"event inside waitFor": {
			body: `await waitFor(() => fireEvent.click(screen.getByRole('button')));`,
			want: "nested or dead closures",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			source := `async function VerifyControls(): Promise<void> { ` + test.body +
				` expect(screen.getByText('Observed outcome')).toBeInTheDocument(); }`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, browserAcceptanceEventSurface(), browserAcceptanceExplicitResult,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func browserAcceptanceEventSurface() directCodingBrowserPublicInteractionSurface {
	return directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
		{Role: "textbox", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Contact", ValueKind: "text"},
		{Role: "spinbutton", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Quantity", ValueKind: "number"},
		{Role: "combobox", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Service", ValueKind: "selection"},
		{Role: "checkbox", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Insured", ValueKind: "boolean"},
		{Role: "radio", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Express", ValueKind: "selection"},
		{Role: "button", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Submit", ValueKind: "action"},
	}}
}
