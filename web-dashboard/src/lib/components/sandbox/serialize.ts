// Discord component type constants
const COMPONENT_TYPE = {
  action_row: 1,
  button: 2,
  string_select: 3,
  text_input: 4,
  user_select: 5,
  role_select: 6,
  mentionable_select: 7,
  channel_select: 8,
  section: 9,
  text_display: 10,
  thumbnail: 11,
  media_gallery: 12,
  file: 13,
  separator: 14,
  container: 17,
} as const;

const BUTTON_STYLE = {
  primary: 1,
  secondary: 2,
  success: 3,
  danger: 4,
  link: 5,
} as const;

export type ButtonStyle = keyof typeof BUTTON_STYLE;

export interface ButtonNode {
  label: string;
  style: ButtonStyle;
  customId?: string;
  url?: string;
  emoji?: string;
}

export type ComponentNode =
  | { type: "text_display"; content: string }
  | {
      type: "section";
      texts: string[];
      accessory?:
        | { kind: "thumbnail"; url: string }
        | {
            kind: "button";
            label: string;
            style: ButtonStyle;
            customId: string;
            url?: string;
            emoji?: string;
          };
    }
  | {
      type: "container";
      accentColor: string;
      spoiler: boolean;
      children: ComponentNode[];
    }
  | {
      type: "separator";
      spacing: "small" | "large";
      divider: boolean;
    }
  | {
      type: "media_gallery";
      items: { url: string; description?: string }[];
    }
  | { type: "action_row"; buttons: ButtonNode[] };

function serializeButton(btn: ButtonNode): Record<string, unknown> {
  const obj: Record<string, unknown> = {
    type: COMPONENT_TYPE.button,
    style: BUTTON_STYLE[btn.style],
    label: btn.label,
  };
  if (btn.style === "link" && btn.url) {
    obj.url = btn.url;
  } else if (btn.customId) {
    obj.custom_id = btn.customId;
  }
  if (btn.emoji) {
    // Custom emoji: <:name:id> or <a:name:id>
    const customMatch = btn.emoji.match(/^<(a?):(\w+):(\d+)>$/);
    if (customMatch) {
      obj.emoji = { name: customMatch[2], id: customMatch[3] };
    } else {
      // Unicode emoji
      obj.emoji = { name: btn.emoji };
    }
  }
  return obj;
}

function hexToDecimal(hex: string): number | undefined {
  if (!hex || hex === "#000000" || hex === "") return undefined;
  const clean = hex.replace("#", "");
  const num = parseInt(clean, 16);
  return isNaN(num) ? undefined : num;
}

function serializeComponent(
  node: ComponentNode,
): Record<string, unknown> | null {
  switch (node.type) {
    case "text_display":
      return {
        type: COMPONENT_TYPE.text_display,
        content: node.content,
      };

    case "section": {
      const components = node.texts
        .filter((t) => t.trim())
        .map((t) => ({
          type: COMPONENT_TYPE.text_display,
          content: t,
        }));

      if (components.length === 0) return null;

      const obj: Record<string, unknown> = {
        type: COMPONENT_TYPE.section,
        components,
      };

      if (node.accessory) {
        if (node.accessory.kind === "thumbnail") {
          obj.accessory = {
            type: COMPONENT_TYPE.thumbnail,
            media: { url: node.accessory.url },
          };
        } else if (node.accessory.kind === "button") {
          obj.accessory = serializeButton({
            label: node.accessory.label,
            style: node.accessory.style,
            customId: node.accessory.customId,
            url: node.accessory.url,
            emoji: node.accessory.emoji,
          });
        }
      }

      return obj;
    }

    case "container": {
      const children = node.children
        .map(serializeComponent)
        .filter((c): c is Record<string, unknown> => c !== null);

      if (children.length === 0) return null;

      const obj: Record<string, unknown> = {
        type: COMPONENT_TYPE.container,
        components: children,
      };

      const color = hexToDecimal(node.accentColor);
      if (color !== undefined) {
        obj.accent_color = color;
      }
      if (node.spoiler) {
        obj.spoiler = true;
      }

      return obj;
    }

    case "separator":
      return {
        type: COMPONENT_TYPE.separator,
        spacing: node.spacing === "large" ? 2 : 1,
        divider: node.divider,
      };

    case "media_gallery": {
      const items = node.items
        .filter((i) => i.url.trim())
        .map((i) => {
          const item: Record<string, unknown> = {
            media: { url: i.url },
          };
          if (i.description) {
            item.description = i.description;
          }
          return item;
        });

      if (items.length === 0) return null;

      return {
        type: COMPONENT_TYPE.media_gallery,
        items,
      };
    }

    case "action_row": {
      const components = node.buttons
        .filter((b) => b.label.trim())
        .map(serializeButton);

      if (components.length === 0) return null;

      return {
        type: COMPONENT_TYPE.action_row,
        components,
      };
    }

    default:
      return null;
  }
}

const BUTTON_STYLE_REVERSE: Record<number, ButtonStyle> = {
  1: "primary",
  2: "secondary",
  3: "success",
  4: "danger",
  5: "link",
};

function decimalToHex(num: number | undefined): string {
  if (num === undefined || num === 0) return "";
  return "#" + num.toString(16).padStart(6, "0");
}

function deserializeButton(obj: Record<string, unknown>): ButtonNode {
  const style = BUTTON_STYLE_REVERSE[(obj.style as number) ?? 1] ?? "primary";
  const btn: ButtonNode = {
    label: (obj.label as string) ?? "",
    style,
  };
  if (obj.custom_id) btn.customId = obj.custom_id as string;
  if (obj.url) btn.url = obj.url as string;
  if (obj.emoji) {
    const emoji = obj.emoji as Record<string, unknown>;
    if (emoji.id) {
      btn.emoji = `<:${emoji.name}:${emoji.id}>`;
    } else if (emoji.name) {
      btn.emoji = emoji.name as string;
    }
  }
  return btn;
}

function deserializeComponent(obj: Record<string, unknown>): ComponentNode | null {
  const type = obj.type as number;

  switch (type) {
    case COMPONENT_TYPE.text_display:
      return { type: "text_display", content: (obj.content as string) ?? "" };

    case COMPONENT_TYPE.section: {
      const components = (obj.components as Record<string, unknown>[]) ?? [];
      const texts = components
        .filter((c) => (c.type as number) === COMPONENT_TYPE.text_display)
        .map((c) => (c.content as string) ?? "");
      if (texts.length === 0) texts.push("");

      const node: ComponentNode = { type: "section", texts };

      if (obj.accessory) {
        const acc = obj.accessory as Record<string, unknown>;
        if ((acc.type as number) === COMPONENT_TYPE.thumbnail) {
          const media = acc.media as Record<string, unknown> | undefined;
          (node as any).accessory = {
            kind: "thumbnail",
            url: (media?.url as string) ?? "",
          };
        } else if ((acc.type as number) === COMPONENT_TYPE.button) {
          const btn = deserializeButton(acc);
          (node as any).accessory = {
            kind: "button",
            label: btn.label,
            style: btn.style,
            customId: btn.customId ?? "",
            url: btn.url,
            emoji: btn.emoji,
          };
        }
      }

      return node;
    }

    case COMPONENT_TYPE.container: {
      const children = ((obj.components as Record<string, unknown>[]) ?? [])
        .map(deserializeComponent)
        .filter((c): c is ComponentNode => c !== null);

      return {
        type: "container",
        accentColor: decimalToHex(obj.accent_color as number | undefined),
        spoiler: (obj.spoiler as boolean) ?? false,
        children,
      };
    }

    case COMPONENT_TYPE.separator:
      return {
        type: "separator",
        spacing: (obj.spacing as number) === 2 ? "large" : "small",
        divider: (obj.divider as boolean) ?? true,
      };

    case COMPONENT_TYPE.media_gallery: {
      const items = ((obj.items as Record<string, unknown>[]) ?? []).map((item) => {
        const media = item.media as Record<string, unknown> | undefined;
        return {
          url: (media?.url as string) ?? "",
          description: item.description as string | undefined,
        };
      });
      if (items.length === 0) items.push({ url: "" });
      return { type: "media_gallery", items };
    }

    case COMPONENT_TYPE.action_row: {
      const components = (obj.components as Record<string, unknown>[]) ?? [];
      const buttons = components
        .filter((c) => (c.type as number) === COMPONENT_TYPE.button)
        .map(deserializeButton);
      if (buttons.length === 0) return null;
      return { type: "action_row", buttons };
    }

    default:
      return null;
  }
}

export function deserializeComponents(json: string): ComponentNode[] {
  if (!json) return [];
  try {
    const parsed = JSON.parse(json) as Record<string, unknown>[];
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map(deserializeComponent)
      .filter((c): c is ComponentNode => c !== null);
  } catch {
    return [];
  }
}

export function serializeComponents(nodes: ComponentNode[]): string {
  const result = nodes
    .map(serializeComponent)
    .filter((c): c is Record<string, unknown> => c !== null);
  return JSON.stringify(result);
}

export function createDefaultNode(
  type: ComponentNode["type"],
): ComponentNode {
  switch (type) {
    case "text_display":
      return { type: "text_display", content: "" };
    case "section":
      return { type: "section", texts: [""], accessory: undefined };
    case "container":
      return {
        type: "container",
        accentColor: "",
        spoiler: false,
        children: [],
      };
    case "separator":
      return { type: "separator", spacing: "small", divider: true };
    case "media_gallery":
      return { type: "media_gallery", items: [{ url: "" }] };
    case "action_row":
      return {
        type: "action_row",
        buttons: [
          { label: "Button", style: "primary", customId: "btn_1" },
        ],
      };
  }
}
