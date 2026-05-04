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
  if (!hex || hex === '#000000' || hex === '') return undefined;
  const num = parseInt(hex.replace('#', ''), 16);
  return isNaN(num) ? undefined : num;
}

function decimalToHex(num) {
  if (num === undefined || num === 0) return '';
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
