<script lang="ts">
  import type { ComponentNode } from "./serialize";

  interface Props {
    node: Extract<ComponentNode, { type: "text_display" }>;
  }

  let { node }: Props = $props();
</script>

<div class="form-control w-full">
  <span class="text-xs font-semibold mb-1">Markdown Content</span>
  <textarea
    class="textarea textarea-bordered w-full"
    rows="3"
    placeholder="**Bold**, *italic*, [links](https://...)"
    bind:value={node.content}
  ></textarea>
</div>
