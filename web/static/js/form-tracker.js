document.addEventListener('alpine:init', () => {
  Alpine.data('formTracker', () => ({
    dirty: false,
    saving: false,
    saved: false,
    _snapshot: [],

    init() {
      this._snapshot = this._capture();
      this.$el.addEventListener('htmx:beforeRequest', () => { this.saving = true; });

      // Reset `saving` when the request completes. After a successful swap
      // the form is replaced and its new Alpine instance starts with
      // saving=false, so these only matter when the existing form remains
      // (network errors, non-HTML 4xx/5xx, beforeSwap cancellations).
      const stopSaving = () => { this.saving = false; };
      this.$el.addEventListener('htmx:afterRequest', stopSaving);
      this.$el.addEventListener('htmx:responseError', stopSaving);
      this.$el.addEventListener('htmx:sendError', stopSaving);

      // Detect server-rendered save success marker
      const marker = this.$el.querySelector('[data-save-success]');
      if (marker) {
        marker.remove();
        this.saved = true;
        setTimeout(() => { this.saved = false; }, 7000);
      }
    },

    // Capture per-element state. Indexed by element rather than name so that
    // duplicate names (e.g. a checkbox paired with a hidden fallback) don't
    // overwrite each other's state.
    _capture() {
      const entries = [];
      for (const el of this.$el.elements) {
        if (!el.name) continue;
        const checkable = el.type === 'checkbox' || el.type === 'radio';
        entries.push({
          el,
          checkable,
          disabled: el.disabled,
          checked: checkable ? el.checked : false,
          value: el.value,
        });
      }
      return entries;
    },

    // Project captured state into the [name, value] pairs the browser would
    // submit. Used for dirty comparison so checkbox/hidden duplicates produce
    // the same shape on both sides.
    _toFormData(entries) {
      const data = [];
      for (const e of entries) {
        if (e.disabled) continue;
        if (e.checkable && !e.checked) continue;
        data.push([e.el.name, e.value]);
      }
      return data;
    },

    checkDirty() {
      this.dirty = JSON.stringify(this._toFormData(this._capture()))
                !== JSON.stringify(this._toFormData(this._snapshot));
    },

    cancel() {
      // Forms with V2 message builder have complex nested Alpine state
      // that can't be restored from hidden input values. Re-fetch from server.
      if (this.$el.querySelector('[x-data*="messageBuilder"]')) {
        const section = this.$el.closest('section');
        if (section && section.id) {
          htmx.ajax('GET', window.location.pathname, { target: '#' + section.id, swap: 'outerHTML' });
          return;
        }
      }

      // Simple forms: restore each element from its captured state.
      for (const e of this._snapshot) {
        if (e.checkable) {
          e.el.checked = e.checked;
        } else {
          e.el.value = e.value;
        }
        e.el.dispatchEvent(new Event('input', { bubbles: true }));
        e.el.dispatchEvent(new Event('change', { bubbles: true }));
      }
      this.dirty = false;
    }
  }));
});
