<script lang="ts">
  import type { ComponentNode, ButtonStyle } from "./serialize";

  interface Props {
    components: ComponentNode[];
  }

  let { components }: Props = $props();

  function btnClass(style: ButtonStyle): string {
    switch (style) {
      case "primary":
        return "btn-primary";
      case "secondary":
        return "btn-neutral";
      case "success":
        return "btn-success";
      case "danger":
        return "btn-error";
      case "link":
        return "btn-link";
    }
  }
</script>

{#snippet previewNode(node: ComponentNode)}
  {#if node.type === "text_display"}
    <div class="whitespace-pre-wrap text-sm">{node.content || "\u00A0"}</div>
  {:else if node.type === "section"}
    <div class="flex items-start justify-between gap-3">
      <div class="flex flex-1 flex-col gap-1">
        {#each node.texts.filter((t) => t.trim()) as text}
          <div class="text-sm">{text}</div>
        {/each}
      </div>
      {#if node.accessory?.kind === "thumbnail" && node.accessory.url}
        <img
          src={node.accessory.url}
          alt="Thumbnail"
          class="h-16 w-16 rounded object-cover"
          onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
        />
      {:else if node.accessory?.kind === "button"}
        <button class="btn btn-sm {btnClass(node.accessory.style)}" disabled>
          {#if node.accessory.emoji}<span class="mr-1">{node.accessory.emoji}</span>{/if}
          {node.accessory.label}
        </button>
      {/if}
    </div>
  {:else if node.type === "container"}
    <div
      class="rounded-lg border-l-4 p-3"
      style:border-color={node.accentColor || "oklch(var(--bc) / 0.2)"}
      class:blur-sm={node.spoiler}
    >
      <div class="flex flex-col gap-2">
        {#each node.children as child}
          {@render previewNode(child)}
        {/each}
      </div>
    </div>
  {:else if node.type === "separator"}
    {#if node.divider}
      <hr class={node.spacing === "large" ? "my-3" : "my-1"} />
    {:else}
      <div class={node.spacing === "large" ? "h-6" : "h-2"}></div>
    {/if}
  {:else if node.type === "media_gallery"}
    <div class="flex flex-wrap gap-2">
      {#each node.items.filter((i) => i.url.trim()) as item}
        <div class="relative">
          <img
            src={item.url}
            alt={item.description || "Media"}
            class="h-32 max-w-48 rounded object-cover"
            onerror={(e) => {
              const el = e.currentTarget as HTMLImageElement;
              el.style.display = "none";
              el.nextElementSibling?.classList.remove("hidden");
            }}
          />
          <div class="bg-base-300 hidden flex h-32 w-48 items-center justify-center rounded text-xs">
            Invalid URL
          </div>
        </div>
      {/each}
    </div>
  {:else if node.type === "action_row"}
    <div class="flex flex-wrap gap-2">
      {#each node.buttons.filter((b) => b.label.trim()) as btn}
        <button class="btn btn-sm {btnClass(btn.style)}" disabled>
          {#if btn.emoji}<span class="mr-1">{btn.emoji}</span>{/if}
          {btn.label}
        </button>
      {/each}
    </div>
  {/if}
{/snippet}

<div class="card bg-base-100 border-base-300 border">
  <div class="card-body gap-3 p-4">
    <h3 class="text-sm font-semibold opacity-60">Preview</h3>
    {#if components.length === 0}
      <div class="text-base-content/40 text-center text-sm">Nothing to preview</div>
    {:else}
      <div class="bg-base-200 flex flex-col gap-2 rounded-lg p-4">
        {#each components as node}
          {@render previewNode(node)}
        {/each}
      </div>
    {/if}
  </div>
</div>
