<script lang="ts">
  interface Props {
    label: string;
    description?: string;
    value: string;
    placeholder?: string;
    onchange: (value: string) => void;
  }

  let { label, description, value, placeholder = "Channel or Role ID", onchange }: Props = $props();
</script>

<label class="flex items-center justify-between gap-4">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>
  <input
    type="text"
    class="input input-bordered w-full max-w-xs shrink-0 font-mono"
    {placeholder}
    {value}
    oninput={(e) => onchange(e.currentTarget.value)}
  />
</label>
