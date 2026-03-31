document.addEventListener('alpine:init', () => {
  Alpine.data('formTracker', () => ({
    dirty: false,
    saving: false,
    saved: false,
    _snapshot: {},

    init() {
      this._snapshot = this._serialize();
      this.$el.addEventListener('htmx:beforeRequest', () => { this.saving = true; });

      // Detect server-rendered save success marker
      const marker = this.$el.querySelector('[data-save-success]');
      if (marker) {
        marker.remove();
        this.saved = true;
        setTimeout(() => { this.saved = false; }, 7000);
      }
    },

    _serialize() {
      const data = {};
      for (const el of this.$el.elements) {
        if (!el.name || el.disabled) continue;
        if (el.type === 'radio' && !el.checked) continue;
        data[el.name] = el.type === 'checkbox' ? el.checked : el.value;
      }
      return data;
    },

    checkDirty() {
      const current = this._serialize();
      this.dirty = JSON.stringify(current) !== JSON.stringify(this._snapshot);
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

      // Simple forms: restore from snapshot
      for (const el of this.$el.elements) {
        if (!el.name || el.disabled || !(el.name in this._snapshot)) continue;
        if (el.type === 'checkbox') {
          el.checked = this._snapshot[el.name];
        } else {
          el.value = this._snapshot[el.name];
        }
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
      }
      this.dirty = false;
    }
  }));
});
