<script lang="ts">
  import type { ComponentNode } from "./serialize";
  import { createDefaultNode } from "./serialize";
  import ComponentEditor from "./ComponentEditor.svelte";

  interface Props {
    components: ComponentNode[];
  }

  let { components = $bindable() }: Props = $props();

  const componentTypes: { value: ComponentNode["type"]; label: string }[] = [
    { value: "text_display", label: "Text Display" },
    { value: "section", label: "Section" },
    { value: "container", label: "Container" },
    { value: "separator", label: "Separator" },
    { value: "media_gallery", label: "Media Gallery" },
    { value: "action_row", label: "Action Row" },
  ];

  function closeDropdown() {
    const el = document.activeElement as HTMLElement | null;
    el?.blur();
  }

  function addComponent(type: ComponentNode["type"]) {
    components = [...components, createDefaultNode(type)];
    closeDropdown();
  }

  function removeComponent(index: number) {
    components = components.filter((_, i) => i !== index);
  }

  function moveComponent(index: number, dir: -1 | 1) {
    const target = index + dir;
    if (target < 0 || target >= components.length) return;
    const arr = [...components];
    [arr[index], arr[target]] = [arr[target], arr[index]];
    components = arr;
  }
</script>

<div class="flex flex-col gap-3">
  {#each components as node, i}
    <div class="card bg-base-100 border-base-300 border shadow-sm">
      <div class="card-body gap-3 p-4">
        <div class="flex items-center justify-between">
          <span class="badge badge-outline">{node.type.replace("_", " ")}</span>
          <div class="flex gap-1">
            <button
              class="btn btn-ghost btn-xs"
              onclick={() => moveComponent(i, -1)}
              disabled={i === 0}
              title="Move up"
            >
              &uarr;
            </button>
            <button
              class="btn btn-ghost btn-xs"
              onclick={() => moveComponent(i, 1)}
              disabled={i === components.length - 1}
              title="Move down"
            >
              &darr;
            </button>
            <button class="btn btn-ghost btn-xs text-error" onclick={() => removeComponent(i)} title="Remove">
              &times;
            </button>
          </div>
        </div>
        <ComponentEditor {node} />
      </div>
    </div>
  {/each}

  {#if components.length === 0}
    <div class="text-base-content/50 py-8 text-center text-sm">
      No components yet. Add one below to get started.
    </div>
  {/if}

  <div class="dropdown">
    <div tabindex="0" role="button" class="btn btn-outline btn-sm">+ Add Component</div>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <ul tabindex="0" class="dropdown-content menu bg-base-300 rounded-box z-10 mt-1 w-52 p-2 shadow">
      {#each componentTypes as ct}
        <li>
          <button onclick={() => addComponent(ct.value)}>{ct.label}</button>
        </li>
      {/each}
    </ul>
  </div>
</div>
