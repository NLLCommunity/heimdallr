<script lang="ts">
  import type { ComponentNode } from "./serialize";

  interface Props {
    node: Extract<ComponentNode, { type: "separator" }>;
  }

  let { node }: Props = $props();
</script>

<div class="flex flex-wrap items-center gap-4">
  <label class="flex items-center gap-2">
    <span class="text-xs font-semibold">Spacing</span>
    <select class="select select-bordered select-sm" bind:value={node.spacing}>
      <option value="small">Small</option>
      <option value="large">Large</option>
    </select>
  </label>
  <label class="flex items-center gap-2">
    <input type="checkbox" class="toggle toggle-sm" bind:checked={node.divider} />
    <span class="text-xs">Show divider line</span>
  </label>
</div>
