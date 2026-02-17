<script lang="ts">
  interface Props {
    label: string;
    description?: string;
    value: number;
    min?: number;
    max?: number;
    step?: number;
    onchange: (value: number) => void;
  }

  let { label, description, value, min, max, step = 1, onchange }: Props = $props();
</script>

<label class="flex items-center justify-between gap-4">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>
  <input
    type="number"
    class="input input-bordered w-full max-w-xs shrink-0"
    {value}
    {min}
    {max}
    {step}
    oninput={(e) => onchange(Number(e.currentTarget.value))}
  />
</label>
