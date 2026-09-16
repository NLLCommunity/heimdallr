// Bridge Lit's mutable plain-JSON draft to Alpine's page state. The bundle is
// fetched only on pages containing an editor, once per page (including swaps).
(() => {
  let modulePromise;
  const load = () => modulePromise ||= import('/static/vendor/components/components.mjs');
  const activeEditors = root => Array.from(root.querySelectorAll('[data-component-editor]'))
    .filter(el => !el.closest('[data-editor-enabled="false"]'));

  window.HeimdallrEditors = {
    ready(root) {
      return activeEditors(root).every(el => el.dataset.editorState === 'ready');
    },
  };

  // Settings editors live inside their form; sandbox actions use a sibling
  // form marked data-editor-submit. V2-off forms remain usable.
  document.addEventListener('submit', event => {
    const form = event.target;
    const root = form.hasAttribute('data-editor-submit')
      ? form.closest('[data-editor-scope]') : form;
    if (root && !window.HeimdallrEditors.ready(root)) {
      event.preventDefault();
      event.stopImmediatePropagation();
      const status = activeEditors(root).find(el => el.dataset.editorState !== 'ready')
        ?.querySelector('[data-editor-status]');
      status?.focus();
    }
  }, true);

  document.addEventListener('alpine:init', () => {
    Alpine.directive('component-editor', (el, { expression }, { evaluate, evaluateLater, effect, cleanup }) => {
      const editor = el.querySelector('discord-message-editor');
      const preview = el.querySelector('discord-message-preview');
      const status = el.querySelector('[data-editor-status]');
      let disposed = false;
      let loaded = false;
      let serialized;
      let latest;

      function setStatus(state, text = '') {
        el.dataset.editorState = state;
        status.textContent = text;
        status.hidden = !text;
      }

      function receive(value) {
        if (disposed) return;
        if (!Array.isArray(value)) {
          latest = undefined;
          setStatus('error', 'Could not load this message. Reload the page before saving.');
          return;
        }
        latest = JSON.stringify(value);
        if (!loaded) return;
        if (serialized !== latest) {
          // Never give Lit an Alpine proxy or let it silently mutate Alpine.
          editor.dataMessage = { flags: 32768, components: JSON.parse(latest) };
          preview.dataMessage = editor.dataMessage;
          serialized = latest;
        }
        setStatus('ready');
      }

      function changed(event) {
        if (event.target !== editor || disposed) return;
        serialized = JSON.stringify(editor.dataMessage.components);
        evaluate(`${expression} = $components`, { scope: { $components: JSON.parse(serialized) } });
        preview.dataMessage = editor.dataMessage;
        preview.refresh();
        // Hidden inputs update in Alpine's next tick. Notify formTracker after
        // that update, including for button-based add/remove/reordering.
        Alpine.nextTick(() => {
          if (!disposed) el.dispatchEvent(new Event('input', { bubbles: true }));
        });
      }

      setStatus('loading', 'Loading message editor…');
      const read = evaluateLater(expression);
      effect(() => read(receive));
      editor.addEventListener('data-message-change', changed);
      cleanup(() => {
        disposed = true;
        editor.removeEventListener('data-message-change', changed);
      });

      load().then(() => {
        if (disposed) return;
        loaded = true;
        editor.capabilities = {
          componentTypes: [1, 2, 9, 10, 11, 12, 14, 17],
          buttonStyles: [5],
          allowAttachmentRequests: false,
        };
        receive(latest === undefined ? null : JSON.parse(latest));
      }).catch(() => {
        if (!disposed) setStatus('error', 'Message editor could not load. Reload the page to try again.');
      });
    });
  });
})();
