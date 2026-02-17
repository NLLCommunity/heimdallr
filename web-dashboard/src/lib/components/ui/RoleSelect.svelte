<script lang="ts">
  import { guildDataStore } from "../../stores/guild-data.svelte";

  interface Props {
    label: string;
    description?: string;
    value: string;
    guildId: string;
    onchange: (value: string) => void;
  }

  let { label, description, value, guildId, onchange }: Props = $props();

  const guildData = guildDataStore();

  $effect(() => {
    if (guildId) {
      guildData.load(guildId);
    }
  });

  const sortedRoles = $derived.by(() => {
    return guildData.roles
      .filter((r) => r.position > 0 && !r.managed)
      .sort((a, b) => b.position - a.position);
  });
</script>

<label class="flex items-center justify-between gap-4">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>
  {#if guildData.loading}
    <span class="loading loading-dots loading-sm"></span>
  {:else}
    <select
      class="select select-bordered w-full max-w-xs shrink-0"
      {value}
      onchange={(e) => onchange(e.currentTarget.value)}
    >
      <option value="">None</option>
      {#each sortedRoles as role}
        <option value={role.id}>{role.name}</option>
      {/each}
    </select>
  {/if}
</label>
