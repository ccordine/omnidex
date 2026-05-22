import { useMemo, useState } from 'react';
import './App.css';

const examples = [
  { expression: 'x^2', operation: 'derivative', result: '2x', steps: ['Use the power rule d/dx x^n = n*x^(n-1).', 'For n=2, d/dx x^2 = 2x.'] },
  { expression: 'sin(x)', operation: 'derivative', result: 'cos(x)', steps: ['Use the standard trig derivative.', 'd/dx sin(x) = cos(x).'] },
  { expression: 'x^2', operation: 'integral', result: 'x^3/3 + C', steps: ['Raise the power by one.', 'Divide by the new power: x^3/3 + C.'] },
  { expression: 'cos(x)', operation: 'integral', result: 'sin(x) + C', steps: ['Find a function whose derivative is cos(x).', 'd/dx sin(x) = cos(x).'] }
];

const localRules = new Map(examples.map((item) => [item.operation + ':' + item.expression, item]));
localRules.set('derivative:x^3', { expression: 'x^3', operation: 'derivative', result: '3x^2', steps: ['Use the power rule.', 'For n=3, d/dx x^3 = 3x^2.'] });
localRules.set('integral:x', { expression: 'x', operation: 'integral', result: 'x^2/2 + C', steps: ['Use the antiderivative power rule.', 'Integral of x is x^2/2 + C.'] });
localRules.set('derivative:e^x', { expression: 'e^x', operation: 'derivative', result: 'e^x', steps: ['The natural exponential is its own derivative.'] });
localRules.set('integral:e^x', { expression: 'e^x', operation: 'integral', result: 'e^x + C', steps: ['The natural exponential is its own antiderivative.'] });

function normalize(value) {
  return value.trim().toLowerCase().replaceAll(' ', '');
}

function App() {
  const [expression, setExpression] = useState('x^2');
  const [operation, setOperation] = useState('derivative');
  const [result, setResult] = useState(examples[0]);
  const [status, setStatus] = useState('Ready');

  const supported = useMemo(() => Array.from(new Set(Array.from(localRules.values()).map((item) => item.expression))).sort(), []);

  async function solve(event) {
    event.preventDefault();
    const payload = { expression: normalize(expression), operation };
    try {
      const response = await fetch('http://127.0.0.1:8080/api/solve', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (!response.ok) throw new Error(await response.text());
      setResult(await response.json());
      setStatus('Solved by Go API');
    } catch (error) {
      const fallback = localRules.get(operation + ':' + payload.expression);
      if (fallback) {
        setResult(fallback);
        setStatus('Solved locally; start the Go API for live backend responses.');
      } else {
        setStatus('Unsupported expression. Try: ' + supported.join(', '));
      }
    }
  }

  function loadExample(example) {
    setExpression(example.expression);
    setOperation(example.operation);
    setResult(example);
    setStatus('Example loaded');
  }

  return (
    <main className="app-shell">
      <section className="workspace" aria-label="Calculus solver">
        <div className="solver-panel">
          <p className="eyebrow">Go API + React</p>
          <h1>Calculus Studio</h1>
          <form onSubmit={solve} className="solver-form">
            <label>
              Expression
              <input value={expression} onChange={(event) => setExpression(event.target.value)} aria-label="Expression" />
            </label>
            <div className="segments" role="group" aria-label="Operation">
              <button type="button" className={operation === 'derivative' ? 'active' : ''} onClick={() => setOperation('derivative')}>Derivative</button>
              <button type="button" className={operation === 'integral' ? 'active' : ''} onClick={() => setOperation('integral')}>Integral</button>
            </div>
            <button className="primary" type="submit">Solve</button>
          </form>
          <p className="status">{status}</p>
        </div>
        <div className="result-panel">
          <p className="eyebrow">Result</p>
          <h2>{result.result}</h2>
          <ol>{result.steps.map((step) => <li key={step}>{step}</li>)}</ol>
        </div>
      </section>
      <section className="examples" aria-label="Worked examples">
        {examples.map((example) => (
          <button key={example.operation + example.expression} onClick={() => loadExample(example)}>
            <span>{example.operation}</span>
            <strong>{example.expression}</strong>
            <em>{example.result}</em>
          </button>
        ))}
      </section>
    </main>
  );
}

export default App;
