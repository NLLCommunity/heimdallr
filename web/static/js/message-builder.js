// Discord component type constants
const COMPONENT_TYPE = {
  action_row: 1,
  button: 2,
  section: 9,
  text_display: 10,
  thumbnail: 11,
  media_gallery: 12,
  separator: 14,
  container: 17,
};

const BUTTON_STYLE = { primary: 1, secondary: 2, success: 3, danger: 4, link: 5 };
const BUTTON_STYLE_REVERSE = { 1: 'primary', 2: 'secondary', 3: 'success', 4: 'danger', 5: 'link' };

function hexToDecimal(hex) {
  if (!hex) return undefined;
  const num = parseInt(hex.replace('#', ''), 16);
  return isNaN(num) ? undefined : num;
}

function decimalToHex(num) {
  if (num === undefined) return '';
  return '#' + num.toString(16).padStart(6, '0');
}

function serializeButton(btn) {
  // In V2 components, only link buttons are meaningful (no event handlers).
  const obj = { type: COMPONENT_TYPE.button, style: BUTTON_STYLE.link, label: btn.label };
  if (btn.url) obj.url = btn.url;
  if (btn.emoji) {
    const m = btn.emoji.match(/^<(a?):(\w+):(\d+)>$/);
    if (m) { obj.emoji = { name: m[2], id: m[3] }; }
    else { obj.emoji = { name: btn.emoji }; }
  }
  return obj;
}

function serializeComponent(node) {
  switch (node.type) {
    case 'text_display':
      return { type: COMPONENT_TYPE.text_display, content: node.content };
    case 'section': {
      const comps = node.texts.filter(t => t.trim()).map(t => ({ type: COMPONENT_TYPE.text_display, content: t }));
      if (!comps.length) return null;
      const obj = { type: COMPONENT_TYPE.section, components: comps };
      if (node.accessory) {
        if (node.accessory.kind === 'thumbnail' && node.accessory.url) {
          obj.accessory = { type: COMPONENT_TYPE.thumbnail, media: { url: node.accessory.url } };
        } else if (node.accessory.kind === 'button' && node.accessory.label) {
          obj.accessory = serializeButton(node.accessory);
        }
      }
      return obj;
    }
    case 'container': {
      const children = node.children.map(serializeComponent).filter(Boolean);
      if (!children.length) return null;
      const obj = { type: COMPONENT_TYPE.container, components: children };
      const color = hexToDecimal(node.accentColor);
      if (color !== undefined) obj.accent_color = color;
      if (node.spoiler) obj.spoiler = true;
      return obj;
    }
    case 'separator':
      return { type: COMPONENT_TYPE.separator, spacing: node.spacing === 'large' ? 2 : 1, divider: node.divider };
    case 'media_gallery': {
      const items = node.items.filter(i => i.url.trim()).map(i => {
        const item = { media: { url: i.url } };
        if (i.description) item.description = i.description;
        return item;
      });
      if (!items.length) return null;
      return { type: COMPONENT_TYPE.media_gallery, items };
    }
    case 'action_row': {
      const comps = node.buttons.filter(b => b.label.trim()).map(serializeButton);
      if (!comps.length) return null;
      return { type: COMPONENT_TYPE.action_row, components: comps };
    }
    default: return null;
  }
}

function deserializeButton(obj) {
  const btn = { label: obj.label ?? '', url: obj.url ?? '' };
  if (obj.emoji) {
    if (obj.emoji.id) btn.emoji = '<:' + obj.emoji.name + ':' + obj.emoji.id + '>';
    else if (obj.emoji.name) btn.emoji = obj.emoji.name;
  }
  return btn;
}

function deserializeComponent(obj) {
  switch (obj.type) {
    case COMPONENT_TYPE.text_display:
      return { type: 'text_display', content: obj.content ?? '' };
    case COMPONENT_TYPE.section: {
      const texts = (obj.components ?? []).filter(c => c.type === COMPONENT_TYPE.text_display).map(c => c.content ?? '');
      if (!texts.length) texts.push('');
      const node = { type: 'section', texts, accessory: null };
      if (obj.accessory) {
        if (obj.accessory.type === COMPONENT_TYPE.thumbnail) {
          node.accessory = { kind: 'thumbnail', url: obj.accessory.media?.url ?? '' };
        } else if (obj.accessory.type === COMPONENT_TYPE.button) {
          const btn = deserializeButton(obj.accessory);
          node.accessory = { kind: 'button', label: btn.label, url: btn.url, emoji: btn.emoji };
        }
      }
      return node;
    }
    case COMPONENT_TYPE.container: {
      const children = (obj.components ?? []).map(deserializeComponent).filter(Boolean);
      return { type: 'container', accentColor: decimalToHex(obj.accent_color), spoiler: obj.spoiler ?? false, children };
    }
    case COMPONENT_TYPE.separator:
      return { type: 'separator', spacing: obj.spacing === 2 ? 'large' : 'small', divider: obj.divider ?? true };
    case COMPONENT_TYPE.media_gallery: {
      const items = (obj.items ?? []).map(i => ({ url: i.media?.url ?? '', description: i.description }));
      if (!items.length) items.push({ url: '' });
      return { type: 'media_gallery', items };
    }
    case COMPONENT_TYPE.action_row: {
      const buttons = (obj.components ?? []).filter(c => c.type === COMPONENT_TYPE.button).map(deserializeButton);
      if (!buttons.length) return null;
      return { type: 'action_row', buttons };
    }
    default: return null;
  }
}

function createDefaultNode(type) {
  switch (type) {
    case 'text_display': return { type: 'text_display', content: '' };
    case 'section': return { type: 'section', texts: [''], accessory: null };
    case 'container': return { type: 'container', accentColor: '', spoiler: false, children: [] };
    case 'separator': return { type: 'separator', spacing: 'small', divider: true };
    case 'media_gallery': return { type: 'media_gallery', items: [{ url: '' }] };
    case 'action_row': return { type: 'action_row', buttons: [{ label: 'Link', url: '' }] };
  }
}

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
      // Initial JSON arrives via the `data-initial` attribute on the host
      // element. Templ's built-in HTML-attribute escaping makes that path
      // safe; we just JSON.parse here. Malformed JSON falls back to an
      // empty editor — same behavior as before.
      const raw = this.$el.dataset.initial;
      if (typeof raw === 'string' && raw) {
        try {
          const parsed = JSON.parse(raw);
          if (Array.isArray(parsed)) {
            this.components = parsed.map(deserializeComponent).filter(Boolean);
          }
        } catch (e) { /* empty */ }
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
          this.components = (data.components || []).map(deserializeComponent).filter(Boolean);
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
    addComponent(type) {
      this.components.push(createDefaultNode(type));
    },
    removeComponent(index) {
      this.components.splice(index, 1);
    },
    moveUp(index) {
      if (index > 0) {
        [this.components[index - 1], this.components[index]] = [this.components[index], this.components[index - 1]];
      }
    },
    moveDown(index) {
      if (index < this.components.length - 1) {
        [this.components[index], this.components[index + 1]] = [this.components[index + 1], this.components[index]];
      }
    },
    serialize() {
      return this.components.map(serializeComponent).filter(Boolean);
    },
    // Section text field management (1-3 texts)
    addSectionText(comp) {
      if (comp.texts.length < 3) comp.texts.push('');
    },
    removeSectionText(comp, index) {
      if (comp.texts.length > 1) comp.texts.splice(index, 1);
    },
    // Section accessory management
    setAccessoryType(comp, kind) {
      if (kind === '') {
        comp.accessory = null;
      } else if (kind === 'thumbnail') {
        comp.accessory = { kind: 'thumbnail', url: '' };
      } else if (kind === 'button') {
        comp.accessory = { kind: 'button', label: '', url: '', emoji: '' };
      }
    },
    // Container child management
    addChild(containerIndex, type) {
      if (type !== 'container') {
        this.components[containerIndex].children.push(createDefaultNode(type));
      }
    },
    removeChild(containerIndex, childIndex) {
      this.components[containerIndex].children.splice(childIndex, 1);
    },
    moveChildUp(containerIndex, childIndex) {
      const children = this.components[containerIndex].children;
      if (childIndex > 0) {
        [children[childIndex - 1], children[childIndex]] = [children[childIndex], children[childIndex - 1]];
      }
    },
    moveChildDown(containerIndex, childIndex) {
      const children = this.components[containerIndex].children;
      if (childIndex < children.length - 1) {
        [children[childIndex], children[childIndex + 1]] = [children[childIndex + 1], children[childIndex]];
      }
    },
  }));
});
