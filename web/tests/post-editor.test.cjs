const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

function makeEditor(documentValue) {
  let factory;
  const context = vm.createContext({
    document: { addEventListener: (_event, init) => init() },
    Alpine: { data: (_name, value) => { factory = value; } },
    setTimeout, clearTimeout, AbortController, URLSearchParams,
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../static/js/post-editor.js'), 'utf8'), context);
  const editor = factory();
  editor.$el = {
    dataset: { initial: JSON.stringify(documentValue), version: '2', name: 'Post' },
    querySelector: () => null,
    addEventListener() {},
  };
  editor.$watch = () => {};
  editor.init();
  return editor;
}

const message = content => ({ components: [{ type: 10, content }] });
const plain = value => JSON.parse(JSON.stringify(value));

test('explicit messages retain their boundaries and fields when serialized', () => {
  const source = { version: 1, messages: [message('One'), message('Two')] };
  source.messages[0].components[0].id = 17;
  const editor = makeEditor(source);
  assert.deepEqual(plain(editor.serialize()), source);
});

test('moving and removing messages preserves the remaining message identities', () => {
  const editor = makeEditor({ version: 1, messages: [message('One'), message('Two')] });
  const firstKey = editor.messages[0].key;
  const secondKey = editor.messages[1].key;
  editor.moveMessage(1, -1);
  assert.equal(editor.messages[0].key, secondKey);
  editor.removeMessage(1);
  editor.addMessage();
  assert.equal(editor.messages[0].key, secondKey);
  assert.notEqual(editor.messages[1].key, firstKey);
  assert.deepEqual(plain(editor.serialize()), { version: 1, messages: [message('Two'), { components: [] }] });
});

test('malformed initial documents are surfaced instead of becoming empty drafts', () => {
  const editor = makeEditor({ version: 9, messages: [message('Keep me')] });
  assert.equal(editor.initialError, true);
  assert.match(editor.message, /load/i);
});

test('post dirty state includes message edits, ordering, name and channel', () => {
  const editor = makeEditor({ version: 1, messages: [message('One'), message('Two')] });
  assert.equal(editor.dirty, false);
  editor.messages[0].components[0].content = 'Changed';
  assert.equal(editor.dirty, true);
  editor.messages[0].components[0].content = 'One';
  assert.equal(editor.dirty, false);
  editor.moveMessage(1, -1);
  assert.equal(editor.dirty, true);
  editor.moveMessage(1, -1);
  editor.name = 'New name';
  assert.equal(editor.dirty, true);
  editor.name = 'Post';
  editor.channelId = '123';
  assert.equal(editor.dirty, true);
});
