import { readFileSync } from 'node:fs';

const app = readFileSync('src/App.js', 'utf8');
const css = readFileSync('src/App.css', 'utf8');
const combined = `${app}\n${css}`.toLowerCase();
const required = [
  "studio",
  "sequencer",
  "channel rack",
  "mixer",
  "transport",
  "tempo",
  "step",
  "channel",
  "play",
  "stop",
  "piano roll",
  "piano",
  "note grid",
  "note",
  "pad",
  "sample",
  "instrument",
  "timeline",
];
const missing = required.filter((term) => !combined.includes(term));
if (missing.length > 0) {
  console.error(`Missing required studio signals: ${missing.join(', ')}`);
  process.exit(1);
}
const hasRange = combined.includes('type="range"') || combined.includes("type: 'range'") || combined.includes('type: "range"');
if (!combined.includes('usestate') || !hasRange || !combined.includes('button')) {
  console.error('Studio implementation must include interactive React controls.');
  process.exit(1);
}
console.log('music studio smoke test passed');
