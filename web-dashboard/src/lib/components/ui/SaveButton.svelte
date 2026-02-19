<script lang="ts">
  interface Props {
    dirty: boolean;
    saving: boolean;
    success: boolean;
    error: string | null;
    onsave: () => void;
  }

  let { dirty, saving, success, error, onsave }: Props = $props();
</script>

<div class="mt-4 flex items-center gap-3">
  <button
    class="btn btn-primary btn-sm"
    disabled={!dirty || saving}
    onclick={onsave}
  >
    {#if saving}
      <span class="loading loading-spinner loading-xs"></span>
      Saving...
    {:else}
      Save
    {/if}
  </button>
  {#if success}
    <span class="text-success text-sm">Saved</span>
  {/if}
  {#if error}
    <span class="text-error text-sm">{error}</span>
  {/if}
</div>
