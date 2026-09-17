const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

function loadFormTracker() {
  let alpineInit;
  let factory;
  const context = {
    Array,
    Event,
    JSON,
    setTimeout,
    document: {
      addEventListener(name, callback) {
        if (name === 'alpine:init') alpineInit = callback;
      },
    },
    Alpine: {
      data(name, value) {
        if (name === 'formTracker') factory = value;
      },
    },
  };
  vm.runInNewContext(fs.readFileSync(__dirname + '/form-tracker.js', 'utf8'), context);
  alpineInit();
  return factory;
}

test('cancel restores the owning form when Alpine $el points at the clicked button', () => {
  const checkbox = {
    name: 'enabled', type: 'checkbox', value: 'true', checked: true, disabled: false,
    dispatchEvent() {},
  };
  const hidden = {
    name: 'enabled', type: 'hidden', value: 'false', disabled: false,
    dispatchEvent() {},
  };
  const form = {
    elements: [checkbox, hidden],
    addEventListener() {},
    querySelector() { return null; },
    closest() { return null; },
  };
  const tracker = loadFormTracker()();
  tracker.$el = form;
  tracker.init();
  checkbox.checked = false;
  tracker.dirty = true;

  tracker.$el = { tagName: 'BUTTON' };
  tracker.cancel();

  assert.equal(checkbox.checked, true);
  assert.equal(tracker.dirty, false);
});
