<script lang="ts">
  interface Props {
    label: string;
    description?: string;
    checked: boolean;
    onchange: (checked: boolean) => void;
  }

  let { label, description, checked, onchange }: Props = $props();
</script>

<div class="form-control">
  <label class="label cursor-pointer justify-start gap-4">
    <input
      type="checkbox"
      class="toggle toggle-primary"
      {checked}
      onchange={(e) => onchange(e.currentTarget.checked)}
    />
    <div>
      <span class="label-text font-medium">{label}</span>
      {#if description}
        <p class="label-text text-base-content/60 text-sm">{description}</p>
      {/if}
    </div>
  </label>
</div>
