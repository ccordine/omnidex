import fs from 'node:fs';

const checks = [
  ['index.html', '/src/main.jsx'],
  ['vite.config.js', '@tailwindcss/vite'],
  ['src/style.css', '@import "tailwindcss"'],
  ['src/App.jsx', 'World Clock'],
  ['src/App.jsx', 'Timezone'],
  ['src/App.jsx', 'America/New_York'],
];

for (const [file, expected] of checks) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(expected)) {
    throw new Error(file + ' missing ' + expected);
  }
}

if (!fs.existsSync('dist/index.html')) {
  throw new Error('dist/index.html missing; run npm run build first');
}

console.log('clock smoke test passed');
