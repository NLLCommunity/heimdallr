// Alpine.js component
document.addEventListener('alpine:init', () => {
  Alpine.data('messageBuilder', () => ({
    components: [],
    // Editor mode:
    //   'create'   — new V2 message (default)
    //   'edit-v2'  — editing an existing V2 message (component editor)
    //   'edit-v1'  — editing an existing V1 message (plain content textarea)
    mode: 'create',
    loadLink: '',
    loadError: '',
    loading: false,
    loadedChannelId: '',
    loadedMessageId: '',
    v1Content: '',
    // Captured in init() because $el rebinds to the event-target element
    // inside event handlers (so this.$el.dataset.loadUrl would be undefined
    // when loadMessage runs from a button click).
    loadUrl: '',
    init() {
      try {
        const parsed = JSON.parse(this.$el.dataset.initial || '[]');
        if (!Array.isArray(parsed)) throw new Error('Expected components');
        this.components = parsed;
      } catch (e) {
        // The adapter shows an error and blocks submission instead of losing data.
        this.components = null;
      }
      this.loadUrl = this.$el.dataset.loadUrl || '';
    },
    async loadMessage() {
      if (!this.loadUrl || !this.loadLink.trim()) return;
      this.loading = true;
      this.loadError = '';
      try {
        const resp = await fetch(this.loadUrl, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'X-Requested-With': 'XMLHttpRequest',
          },
          body: new URLSearchParams({ link: this.loadLink }),
        });
        if (resp.status === 401) {
          window.location.assign('/login');
          return;
        }
        // The handler returns JSON on its own paths, but checkGuildAdmin /
        // rate-limit middleware emit text/plain via http.Error. Branch on
        // Content-Type so a 403 from those paths surfaces the actual reason
        // instead of the generic JSON-parse fallback.
        const ct = resp.headers.get('content-type') || '';
        const isJSON = ct.includes('application/json');
        const data = isJSON ? await resp.json().catch(() => null) : null;
        if (!resp.ok) {
          if (data && data.error) {
            this.loadError = data.error;
          } else {
            const text = isJSON ? '' : (await resp.text().catch(() => ''));
            this.loadError = text.trim() || 'Failed to load message.';
          }
          return;
        }
        if (!data) {
          this.loadError = 'Unexpected response from server.';
          return;
        }
        this.loadedChannelId = data.channel_id || '';
        this.loadedMessageId = data.message_id || '';
        if (data.is_v2) {
          this.mode = 'edit-v2';
          this.v1Content = '';
          this.components = Array.isArray(data.components) ? data.components : null;
        } else {
          this.mode = 'edit-v1';
          this.v1Content = data.content || '';
          this.components = [];
        }
      } catch (e) {
        this.loadError = 'Network error while loading message.';
      } finally {
        this.loading = false;
      }
    },
    resetToCreate() {
      this.mode = 'create';
      this.loadedChannelId = '';
      this.loadedMessageId = '';
      this.v1Content = '';
      this.components = [];
      this.loadLink = '';
      this.loadError = '';
    },
    serialize() {
      return this.components;
    },
  }));
});
