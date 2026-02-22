<script lang="ts">
  import type { ComponentNode } from "./serialize";

  interface Props {
    node: Extract<ComponentNode, { type: "section" }>;
  }

  let { node }: Props = $props();

  let accessoryType = $derived<"none" | "thumbnail" | "button">(
    node.accessory?.kind ?? "none",
  );

  function setAccessoryType(type: "none" | "thumbnail" | "button") {
    if (type === "none") {
      node.accessory = undefined;
    } else if (type === "thumbnail") {
      node.accessory = { kind: "thumbnail", url: "" };
    } else {
      node.accessory = {
        kind: "button",
        label: "Click",
        style: "link",
        customId: "",
        url: "",
      };
    }
  }

  function addText() {
    node.texts = [...node.texts, ""];
  }

  function removeText(index: number) {
    node.texts = node.texts.filter((_, i) => i !== index);
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex flex-col gap-2">
    <span class="text-xs font-semibold">Text Fields</span>
    {#each node.texts as _, i}
      <div class="flex items-start gap-2">
        <textarea
          class="textarea textarea-bordered textarea-sm flex-1"
          rows="2"
          placeholder="Section text (markdown)"
          bind:value={node.texts[i]}
        ></textarea>
        {#if node.texts.length > 1}
          <button class="btn btn-ghost btn-sm btn-square" onclick={() => removeText(i)} title="Remove">
            &times;
          </button>
        {/if}
      </div>
    {/each}
    {#if node.texts.length < 3}
      <button class="btn btn-ghost btn-xs self-start" onclick={addText}>+ Add text</button>
    {/if}
  </div>

  <div class="flex flex-col gap-2">
    <span class="text-xs font-semibold">Accessory</span>
    <select
      class="select select-bordered select-sm w-40"
      value={accessoryType}
      onchange={(e) => setAccessoryType(e.currentTarget.value as "none" | "thumbnail" | "button")}
    >
      <option value="none">None</option>
      <option value="thumbnail">Thumbnail</option>
      <option value="button">Link Button</option>
    </select>

    {#if node.accessory?.kind === "thumbnail"}
      <input
        type="text"
        class="input input-bordered input-sm"
        placeholder="Thumbnail URL"
        bind:value={node.accessory.url}
      />
    {:else if node.accessory?.kind === "button"}
      <div class="flex items-center gap-2">
        <input
          type="text"
          class="input input-bordered input-sm flex-1"
          placeholder="Label"
          bind:value={node.accessory.label}
        />
        <input
          type="text"
          class="input input-bordered input-sm w-36"
          placeholder="😀 or :name:"
          bind:value={node.accessory.emoji}
        />
      </div>
      <input
        type="text"
        class="input input-bordered input-sm"
        placeholder="https://..."
        bind:value={node.accessory.url}
      />
    {/if}
  </div>
</div>
