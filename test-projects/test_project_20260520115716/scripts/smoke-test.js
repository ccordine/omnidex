const fs = require('fs');
const required = [
  ['index.html', 'data-controller="calculator"'],
  ['index.html', 'dist/bundle.js'],
  ['src/index.js', '@hotwired/stimulus'],
  ['src/index.js', 'recyclrjs'],
  ['src/index.js', 'class CalculatorController'],
  ['src/styles.css', '.calculator'],
  ['dist/bundle.js', 'CalculatorController']
];
for (const [file, needle] of required) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(needle)) throw new Error(file + ' missing ' + needle);
}
console.log('calculator smoke test passed');
