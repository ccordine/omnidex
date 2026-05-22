import { useMemo, useState } from 'react';
import { formatJSON, minifyJSON } from './jsonFormatter.js';

const sample = '{"project":"omnidex","features":["format","minify","validate"],"ok":true}';

export default function App() {
  const [input, setInput] = useState(sample);
  const [mode, setMode] = useState('format');

  const result = useMemo(() => {
    return mode === 'minify' ? minifyJSON(input) : formatJSON(input, 2);
  }, [input, mode]);

  return (
    <main className="app-shell">
      <section className="panel" aria-labelledby="title">
        <div className="header-row">
          <div>
            <p className="eyebrow">React Utility</p>
            <h1 id="title">JSON Formatter</h1>
          </div>
          <div className="actions" aria-label="Formatter actions">
            <button type="button" className={mode === 'format' ? 'active' : ''} onClick={() => setMode('format')}>Format</button>
            <button type="button" className={mode === 'minify' ? 'active' : ''} onClick={() => setMode('minify')}>Minify</button>
          </div>
        </div>

        <div className="workspace">
          <label>
            Input JSON
            <textarea value={input} onChange={(event) => setInput(event.target.value)} spellCheck="false" />
          </label>
          <label>
            Output
            <pre className={result.ok ? 'output' : 'output error'}>{result.ok ? result.value : result.error}</pre>
          </label>
        </div>
      </section>
    </main>
  );
}
