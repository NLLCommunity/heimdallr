<script lang="ts">
  interface Props {
    label: string;
    description?: string;
    value: string;
    placeholder?: string;
    helpText?: string;
    onchange: (value: string) => void;
  }

  let { label, description, value, placeholder, helpText, onchange }: Props = $props();

  let showHelp = $state(false);
</script>

<div class="form-control">
  <label class="label">
    <span class="label-text font-medium">{label}</span>
    {#if description}
      <span class="label-text text-base-content/60 text-sm">{description}</span>
    {/if}
    <textarea
      class="textarea textarea-bordered w-full"
      rows="3"
      {placeholder}
      {value}
      oninput={(e) => onchange(e.currentTarget.value)}
    ></textarea>
  </label>
  {#if helpText}
    <button
      type="button"
      class="btn btn-ghost btn-xs mt-1 self-start"
      onclick={() => (showHelp = !showHelp)}
    >
      {showHelp ? "Hide" : "Show"} template placeholders
    </button>
    {#if showHelp}
      <div class="bg-base-200 mt-1 rounded-lg p-3 text-sm">
        <pre class="whitespace-pre-wrap">{helpText}</pre>
      </div>
    {/if}
  {/if}
</div>
