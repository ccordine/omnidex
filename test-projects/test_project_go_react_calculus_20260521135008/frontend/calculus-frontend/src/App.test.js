import { render, screen } from '@testing-library/react';
import App from './App';

test('renders calculus solver controls and worked result', () => {
  render(<App />);
  expect(screen.getByText(/Calculus Studio/i)).toBeInTheDocument();
  expect(screen.getByLabelText(/Expression/i)).toBeInTheDocument();
  expect(screen.getAllByText('2x').length).toBeGreaterThan(0);
});
