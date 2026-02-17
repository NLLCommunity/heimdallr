<script lang="ts">
  interface Props {
    label: string;
    description?: string;
    checked: boolean;
    onchange: (checked: boolean) => void;
  }

  let { label, description, checked, onchange }: Props = $props();
</script>

<label class="flex cursor-pointer items-center justify-between gap-4">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>
  <input
    type="checkbox"
    class="toggle toggle-primary shrink-0"
    {checked}
    onchange={(e) => onchange(e.currentTarget.checked)}
  />
</label>
