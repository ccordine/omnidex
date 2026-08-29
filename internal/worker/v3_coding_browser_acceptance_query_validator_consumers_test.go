package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceQueriesAllowOnlyDirectGroundedConsumers(t *testing.T) {
	tests := map[string]string{
		"text content forgery": `function Verify(): void {
  screen.getByRole('button', { name: 'Apply adjustment' }).textContent = 'Updated';
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
		"asserted property": `function Verify(): void {
  expect(screen.getByRole('button', { name: 'Apply adjustment' }).textContent).toBe('Apply adjustment');
}`,
		"direct DOM call": `function Verify(): void {
  screen.getByRole('button', { name: 'Apply adjustment' }).click();
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
		"arbitrary call argument": `function Verify(): void {
  observe(screen.getByText('Updated'));
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
		"wrapped interaction target": `function Verify(): void {
  fireEvent.click(select(screen.getByRole('button', { name: 'Apply adjustment' })));
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
		"standalone query": `function Verify(): void {
  screen.getByText('Updated');
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
		"assigned query": `function Verify(): void {
  target = screen.getByRole('button', { name: 'Apply adjustment' });
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source,
				true,
				browserAcceptanceInventorySurface(),
				browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), "must be consumed directly") {
				t.Fatalf("query escape was accepted or returned the wrong error: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceQueriesAcceptOnlyTransparentSelectionWrappers(t *testing.T) {
	source := `async function Verify(): Promise<void> {
  fireEvent.change((screen.getAllByRole('textbox')[0]), { target: { value: 'AX-7' } });
  fireEvent.input(((await screen.findAllByRole('textbox'))[1]), { target: { value: 'L2' } });
  fireEvent.click((await (screen.findByRole('button', { name: 'Apply adjustment' }))));
  expect(((await (screen.findByText('Updated'))))).toBeInTheDocument();
  expect((screen.getByRole('textbox', { name: 'Stock code' }))).toHaveValue('AX-7');
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source,
		true,
		browserAcceptanceInventorySurface(),
		browserAcceptanceNoDerivedResult,
	); err != nil {
		t.Fatalf("transparent direct query wrappers were rejected: %v", err)
	}
}
