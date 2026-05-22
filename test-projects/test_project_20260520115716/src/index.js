const { Application, Controller } = require('@hotwired/stimulus');
require('./styles.css');
const Recyclr = require('recyclrjs');
const recyclrRuntime = Recyclr.default || Recyclr;

const buttons = [
  ['C', 'clear', 'utility'], ['DEL', 'delete', 'utility'], ['%', '%', 'operator'], ['/', '/', 'operator'],
  ['7', '7'], ['8', '8'], ['9', '9'], ['x', '*', 'operator'],
  ['4', '4'], ['5', '5'], ['6', '6'], ['-', '-', 'operator'],
  ['1', '1'], ['2', '2'], ['3', '3'], ['+', '+', 'operator'],
  ['0', '0', 'zero'], ['.', '.'], ['=', 'equals', 'equals']
];

class CalculatorController extends Controller {
  static targets = ['display', 'status', 'keys'];

  connect() {
    this.expression = '';
    this.lastResult = null;
    this.keysTarget.innerHTML = buttons.map(([label, value, type]) => {
      const action = value === 'clear' || value === 'delete' || value === 'equals'
        ? 'click->calculator#' + value
        : 'click->calculator#press';
      const data = value === 'clear' || value === 'delete' || value === 'equals' ? '' : ' data-value="' + value + '"';
      return '<button class="key ' + (type ? 'key--' + type : '') + '" type="button" data-action="' + action + '"' + data + '>' + label + '</button>';
    }).join('');
    this.update('Ready');
    if (recyclrRuntime && typeof recyclrRuntime.mount === 'function') recyclrRuntime.mount(document);
  }

  press(event) { this.add(event.currentTarget.dataset.value || ''); }
  add(token) {
    if (!token) return;
    if (this.lastResult !== null && /[0-9.]/.test(token)) {
      this.expression = '';
      this.lastResult = null;
    }
    if (/[+*/%-]/.test(token) && (this.expression === '' || /[+*/%.-]$/.test(this.expression))) {
      if (token !== '-' || /[-.]$/.test(this.expression)) return;
    }
    if (token === '.' && this.currentNumber().includes('.')) return;
    this.expression += token;
    this.update('Editing');
  }
  delete() { this.expression = this.expression.slice(0, -1); this.lastResult = null; this.update('Deleted'); }
  clear() { this.expression = ''; this.lastResult = null; this.update('Cleared'); }
  equals() {
    if (!this.expression || /[+*/%.-]$/.test(this.expression)) { this.statusTarget.textContent = 'Complete the expression first'; return; }
    try {
      const value = Function('"use strict"; return (' + this.expression + ')')();
      if (!Number.isFinite(value)) throw new Error('Cannot divide by zero');
      this.expression = String(Number.isInteger(value) ? value : Number(value.toFixed(8)));
      this.lastResult = this.expression;
      this.update('Result');
    } catch (error) {
      this.statusTarget.textContent = error.message || 'Invalid expression';
    }
  }
  handleKey(event) {
    if (/^[0-9.]$/.test(event.key) || ['+', '-', '*', '/', '%'].includes(event.key)) { this.add(event.key); event.preventDefault(); }
    else if (event.key === 'Enter' || event.key === '=') { this.equals(); event.preventDefault(); }
    else if (event.key === 'Backspace') { this.delete(); event.preventDefault(); }
    else if (event.key === 'Escape') { this.clear(); event.preventDefault(); }
  }
  currentNumber() { return this.expression.split(/[+*/%-]/).pop() || ''; }
  update(status) { this.displayTarget.textContent = this.expression || '0'; this.statusTarget.textContent = status; }
}

const application = Application.start();
application.register('calculator', CalculatorController);
