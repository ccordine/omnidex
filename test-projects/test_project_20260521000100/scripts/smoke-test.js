import fs from 'node:fs';
import { formatJSON, minifyJSON } from '../src/jsonFormatter.js';

const formatted = formatJSON('{"b":2,"a":[1,true]}');
if (!formatted.ok || !formatted.value.includes('\n  "b": 2') || !formatted.value.includes('\n  "a": [')) {
  throw new Error('formatJSON did not pretty-print nested JSON');
}

const minified = minifyJSON('{"b":2, "a": [1, true]}');
if (!minified.ok || minified.value !== '{"b":2,"a":[1,true]}') {
  throw new Error('minifyJSON did not minify JSON');
}

const invalid = formatJSON('{"broken":');
if (invalid.ok || !invalid.error) {
  throw new Error('invalid JSON did not produce an error');
}

const sourceChecks = [
  ['src/App.jsx', 'JSON Formatter'],
  ['src/App.jsx', 'Input JSON'],
  ['src/App.jsx', 'Minify'],
  ['src/jsonFormatter.js', 'formatJSON'],
  ['src/jsonFormatter.js', 'minifyJSON'],
  ['dist/index.html', '/assets/'],
];
for (const [file, expected] of sourceChecks) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(expected)) throw new Error(file + ' missing ' + expected);
}

console.log('json formatter smoke test passed');
