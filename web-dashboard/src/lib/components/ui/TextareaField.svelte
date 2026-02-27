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

<div class="flex flex-col gap-1">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>
  <textarea
    class="textarea textarea-bordered w-full"
    rows="3"
    {placeholder}
    {value}
    oninput={(e) => onchange(e.currentTarget.value)}
  ></textarea>
  {#if helpText}
    <button
      type="button"
      class="btn btn-ghost btn-xs self-start"
      onclick={() => (showHelp = !showHelp)}
    >
      {showHelp ? "Hide" : "Show"} template placeholders
    </button>
    {#if showHelp}
      <div class="bg-base-200 rounded-lg p-3 text-sm">
        <pre class="whitespace-pre-wrap">{helpText}</pre>
      </div>
    {/if}
  {/if}
</div>
