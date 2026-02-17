<script lang="ts">
  import type { ComponentNode } from "./serialize";
  import { createDefaultNode } from "./serialize";
  import ComponentEditor from "./ComponentEditor.svelte";

  interface Props {
    node: Extract<ComponentNode, { type: "container" }>;
  }

  let { node }: Props = $props();

  const componentTypes: { value: ComponentNode["type"]; label: string }[] = [
    { value: "text_display", label: "Text Display" },
    { value: "section", label: "Section" },
    { value: "separator", label: "Separator" },
    { value: "media_gallery", label: "Media Gallery" },
    { value: "action_row", label: "Action Row" },
  ];

  function closeDropdown() {
    const el = document.activeElement as HTMLElement | null;
    el?.blur();
  }

  function addChild(type: ComponentNode["type"]) {
    node.children = [...node.children, createDefaultNode(type)];
    closeDropdown();
  }

  function removeChild(index: number) {
    node.children = node.children.filter((_, i) => i !== index);
  }

  function moveChild(index: number, dir: -1 | 1) {
    const target = index + dir;
    if (target < 0 || target >= node.children.length) return;
    const arr = [...node.children];
    [arr[index], arr[target]] = [arr[target], arr[index]];
    node.children = arr;
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex flex-wrap items-center gap-3">
    <label class="flex items-center gap-2">
      <span class="text-xs font-semibold">Accent Color</span>
      <input type="color" class="h-8 w-8 cursor-pointer" bind:value={node.accentColor} />
    </label>
    <label class="flex items-center gap-2">
      <input type="checkbox" class="toggle toggle-sm" bind:checked={node.spoiler} />
      <span class="text-xs">Spoiler</span>
    </label>
  </div>

  <div class="border-base-300 flex flex-col gap-2 border-l-2 pl-3">
    <span class="text-xs font-semibold">Children</span>
    {#each node.children as child, i}
      <div class="bg-base-200 rounded-lg p-3">
        <div class="mb-2 flex items-center justify-between">
          <span class="badge badge-sm badge-outline">{child.type.replace("_", " ")}</span>
          <div class="flex gap-1">
            <button class="btn btn-ghost btn-xs" onclick={() => moveChild(i, -1)} disabled={i === 0}>
              &uarr;
            </button>
            <button
              class="btn btn-ghost btn-xs"
              onclick={() => moveChild(i, 1)}
              disabled={i === node.children.length - 1}
            >
              &darr;
            </button>
            <button class="btn btn-ghost btn-xs" onclick={() => removeChild(i)}>&times;</button>
          </div>
        </div>
        <ComponentEditor node={child} />
      </div>
    {/each}

    <div class="dropdown">
      <div tabindex="0" role="button" class="btn btn-ghost btn-xs">+ Add child component</div>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <ul tabindex="0" class="dropdown-content menu bg-base-300 rounded-box z-10 w-48 p-2 shadow">
        {#each componentTypes as ct}
          <li>
            <button onclick={() => addChild(ct.value)}>{ct.label}</button>
          </li>
        {/each}
      </ul>
    </div>
  </div>
</div>
