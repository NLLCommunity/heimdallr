// post-editor.js — Alpine factory backing /posts/{id} editor pages.
// Reuses serializeComponent / deserializeComponent / createDefaultNode
// (defined globally in message-builder.js).

document.addEventListener('alpine:init', () => {
  Alpine.data('postEditor', () => ({
    components: [],
    name: '',
    channelId: '',
    version: 0,
    busy: false,
    message: '',
    saveUrl: '',
    publishUrl: '',
    unpublishUrl: '',
    deleteUrl: '',
    previewUrl: '',
    redirectUrlPrefix: '',
    init() {
      const ds = this.$el.dataset;
      this.name = ds.name || '';
      this.channelId = ds.channelId || '';
      this.version = parseInt(ds.version || '0', 10);
      this.saveUrl = ds.saveUrl || '';
      this.publishUrl = ds.publishUrl || '';
      this.unpublishUrl = ds.unpublishUrl || '';
      this.deleteUrl = ds.deleteUrl || '';
      this.previewUrl = ds.previewUrl || '';
      this.redirectUrlPrefix = ds.redirectUrlPrefix || '';
      const raw = ds.initial || '[]';
      try {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) {
          this.components = parsed.map(deserializeComponent).filter(Boolean);
        }
      } catch (e) { /* empty */ }
      this.$watch('components', () => this.refreshPreview(), { deep: true });
      this.refreshPreview();

      const sel = this.$el.querySelector('select[name="channel_id"]');
      if (sel) {
        this.channelId = sel.value;
        sel.addEventListener('change', () => { this.channelId = sel.value; });
      }
    },
    addComponent(type) { this.components.push(createDefaultNode(type)); },
    removeComponent(i) { this.components.splice(i, 1); },
    moveUp(i) { if (i > 0) [this.components[i-1], this.components[i]] = [this.components[i], this.components[i-1]]; },
    moveDown(i) { if (i < this.components.length - 1) [this.components[i], this.components[i+1]] = [this.components[i+1], this.components[i]]; },
    addSectionText(c) { if (c.texts.length < 3) c.texts.push(''); },
    removeSectionText(c, i) { if (c.texts.length > 1) c.texts.splice(i, 1); },
    setAccessoryType(c, kind) {
      if (kind === '') c.accessory = null;
      else if (kind === 'thumbnail') c.accessory = { kind: 'thumbnail', url: '' };
      else if (kind === 'button') c.accessory = { kind: 'button', label: '', url: '', emoji: '' };
    },
    addChild(ci, type) { if (type !== 'container') this.components[ci].children.push(createDefaultNode(type)); },
    removeChild(ci, ki) { this.components[ci].children.splice(ki, 1); },
    moveChildUp(ci, ki) { const a = this.components[ci].children; if (ki > 0) [a[ki-1], a[ki]] = [a[ki], a[ki-1]]; },
    moveChildDown(ci, ki) { const a = this.components[ci].children; if (ki < a.length - 1) [a[ki], a[ki+1]] = [a[ki+1], a[ki]]; },
    serialize() { return this.components.map(serializeComponent).filter(Boolean); },
    async save() {
      this.busy = true;
      this.message = '';
      try {
        const body = new URLSearchParams({
          version: String(this.version),
          name: this.name,
          components_json: JSON.stringify(this.serialize()),
          channel_id: this.channelId || '',
        });
        const resp = await fetch(this.saveUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body,
        });
        if (resp.status === 409) {
          this.message = 'This post was updated elsewhere. Reload to see the latest version.';
          return;
        }
        if (!resp.ok) {
          this.message = (await resp.text()) || 'Save failed.';
          return;
        }
        const data = await resp.json().catch(() => null);
        // New posts start with version 0 in the data attribute; the server
        // assigned a real ID on first save and the client must navigate so
        // publish/delete/etc. become available. If the response can't be
        // parsed or lacks an id, treat it as an error rather than silently
        // staying on /posts/new — clicking Save again would otherwise insert
        // a second row.
        const isNewPost = this.version === 0;
        if (isNewPost) {
          if (!data || typeof data.id !== 'number' || !this.redirectUrlPrefix) {
            this.message = 'Save returned an unexpected response. Reload the post list to see if it was saved.';
            return;
          }
          window.location.assign(this.redirectUrlPrefix + data.id);
          return;
        }
        // Existing-post save: bump version so the next save doesn't 409.
        if (data && typeof data.version === 'number') {
          this.version = data.version;
        }
        this.message = 'Saved.';
      } finally {
        this.busy = false;
      }
    },
    async publish() { await this._postAction(this.publishUrl, 'Published.'); },
    async unpublish() { await this._postAction(this.unpublishUrl, 'Unpublished from Discord.'); },
    async del() {
      if (!confirm('Delete this post permanently? Discord messages will also be removed.')) return;
      const resp = await fetch(this.deleteUrl, { method: 'POST' });
      if (resp.ok) {
        window.location.assign(this.deleteUrl.replace(/\/posts\/\d+\/delete$/, '/posts'));
      } else {
        this.message = (await resp.text()) || 'Delete failed.';
      }
    },
    async _postAction(url, okMsg) {
      this.busy = true;
      this.message = '';
      try {
        const resp = await fetch(url, { method: 'POST' });
        if (!resp.ok) {
          this.message = (await resp.text()) || 'Request failed.';
          return;
        }
        this.message = okMsg;
      } finally {
        this.busy = false;
      }
    },
    async refreshPreview() {
      if (!this.previewUrl) return;
      try {
        const resp = await fetch(this.previewUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ components_json: JSON.stringify(this.serialize()) }),
        });
        const html = await resp.text();
        const target = document.getElementById('split-preview');
        if (target) target.innerHTML = html;
      } catch (e) { /* swallow */ }
    },
  }));
});
