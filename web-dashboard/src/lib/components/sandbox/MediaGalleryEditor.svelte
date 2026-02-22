<script lang="ts">
  import type { ComponentNode } from "./serialize";

  interface Props {
    node: Extract<ComponentNode, { type: "media_gallery" }>;
  }

  let { node }: Props = $props();

  function addItem() {
    node.items = [...node.items, { url: "" }];
  }

  function removeItem(index: number) {
    node.items = node.items.filter((_, i) => i !== index);
  }
</script>

<div class="flex flex-col gap-2">
  <span class="text-xs font-semibold">Media Items</span>
  {#each node.items as item, i}
    <div class="flex items-center gap-2">
      <input
        type="text"
        class="input input-bordered input-sm flex-1"
        placeholder="https://example.com/image.png"
        bind:value={item.url}
      />
      <input
        type="text"
        class="input input-bordered input-sm w-40"
        placeholder="Description"
        bind:value={item.description}
      />
      {#if node.items.length > 1}
        <button class="btn btn-ghost btn-sm btn-square" onclick={() => removeItem(i)} title="Remove">
          &times;
        </button>
      {/if}
    </div>
  {/each}
  {#if node.items.length < 10}
    <button class="btn btn-ghost btn-xs self-start" onclick={addItem}>+ Add media item</button>
  {/if}
</div>
