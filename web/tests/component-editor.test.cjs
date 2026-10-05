const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

test('submit guard rejects an invalid active editor and accepts it after correction', () => {
  let submit;
  const document = { addEventListener(type, listener) {
    if (type === 'submit') submit = listener;
  } };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../static/js/component-editor.js'), 'utf8'), {
    document, window: {},
  });
  let valid = false;
  const editor = { reportValidity: () => valid };
  const wrapper = {
    dataset: { editorState: 'ready' },
    closest: () => null,
    querySelector: selector => selector === 'discord-message-editor' ? editor : null,
  };
  const form = {
    hasAttribute: () => false,
    querySelectorAll: () => [wrapper],
  };
  const event = {
    target: form,
    prevented: false,
    preventDefault() { this.prevented = true; },
    stopImmediatePropagation() {},
  };
  submit(event);
  assert.equal(event.prevented, true);
  valid = true;
  event.prevented = false;
  submit(event);
  assert.equal(event.prevented, false);
  const disabled = { closest: () => ({}), reportValidity: () => {
    throw new Error('V2-off editor must not be validated');
  } };
  form.querySelectorAll = () => [wrapper, disabled];
  submit(event);
  assert.equal(event.prevented, false);
});
