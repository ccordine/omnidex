const fs = require('fs');
const required = [
  'backend/calculus-api/main.go',
  'backend/calculus-api/calc.go',
  'backend/calculus-api/calc_test.go',
  'frontend/calculus-frontend/src/App.js',
  'frontend/calculus-frontend/src/App.css',
  'frontend/calculus-frontend/src/App.test.js'
];
for (const file of required) {
  if (!fs.existsSync(file) || fs.statSync(file).size === 0) {
    throw new Error(file + ' is missing or empty');
  }
}
const app = fs.readFileSync('frontend/calculus-frontend/src/App.js', 'utf8');
for (const token of ['Calculus Studio', 'derivative', 'integral', 'fetch']) {
  if (!app.includes(token)) throw new Error('missing frontend token ' + token);
}
console.log('go react calculus smoke test passed');
