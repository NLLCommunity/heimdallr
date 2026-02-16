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

<div class="form-control">
  <label class="label">
    <span class="label-text font-medium">{label}</span>
    {#if description}
      <span class="label-text text-base-content/60 text-sm">{description}</span>
    {/if}
    <input
      type="number"
      class="input input-bordered w-full max-w-xs"
      {value}
      {min}
      {max}
      {step}
      oninput={(e) => onchange(Number(e.currentTarget.value))}
    />
  </label>
</div>
