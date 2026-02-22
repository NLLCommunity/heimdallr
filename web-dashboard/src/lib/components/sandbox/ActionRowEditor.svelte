<script lang="ts">
  import type { ComponentNode, ButtonStyle } from "./serialize";

  interface Props {
    node: Extract<ComponentNode, { type: "action_row" }>;
  }

  let { node }: Props = $props();

  const styles: { value: ButtonStyle; label: string }[] = [
    { value: "primary", label: "Primary" },
    { value: "secondary", label: "Secondary" },
    { value: "success", label: "Success" },
    { value: "danger", label: "Danger" },
    { value: "link", label: "Link" },
  ];

  function addButton() {
    node.buttons = [
      ...node.buttons,
      { label: "Button", style: "secondary", customId: `btn_${Date.now()}` },
    ];
  }

  function removeButton(index: number) {
    node.buttons = node.buttons.filter((_, i) => i !== index);
  }
</script>

<div class="flex flex-col gap-3">
  <span class="text-xs font-semibold">Buttons</span>
  {#each node.buttons as btn, i}
    <div class="bg-base-200 flex flex-col gap-2 rounded-lg p-3">
      <div class="flex items-center gap-2">
        <input
          type="text"
          class="input input-bordered input-sm flex-1"
          placeholder="Label"
          bind:value={btn.label}
        />
        <select class="select select-bordered select-sm" bind:value={btn.style}>
          {#each styles as s}
            <option value={s.value}>{s.label}</option>
          {/each}
        </select>
        {#if node.buttons.length > 1}
          <button class="btn btn-ghost btn-sm btn-square" onclick={() => removeButton(i)} title="Remove">
            &times;
          </button>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        {#if btn.style === "link"}
          <input
            type="text"
            class="input input-bordered input-sm flex-1"
            placeholder="https://..."
            bind:value={btn.url}
          />
        {:else}
          <input
            type="text"
            class="input input-bordered input-sm flex-1"
            placeholder="Custom ID"
            bind:value={btn.customId}
          />
        {/if}
        <input
          type="text"
          class="input input-bordered input-sm w-36"
          placeholder="😀 or :name:"
          bind:value={btn.emoji}
        />
      </div>
    </div>
  {/each}
  {#if node.buttons.length < 5}
    <button class="btn btn-ghost btn-xs self-start" onclick={addButton}>+ Add button</button>
  {/if}
</div>
