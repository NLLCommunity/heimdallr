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

<div class="form-control">
  <label class="label">
    <span class="label-text font-medium">{label}</span>
    {#if description}
      <span class="label-text text-base-content/60 text-sm">{description}</span>
    {/if}
    <input
      type="text"
      class="input input-bordered w-full max-w-xs font-mono"
      {placeholder}
      {value}
      oninput={(e) => onchange(e.currentTarget.value)}
    />
  </label>
</div>
